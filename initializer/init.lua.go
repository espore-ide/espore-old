package initializer

const initLua = `
print("\n\n\nEspore bootloader will launch in 3 seconds.")
print("Set main to nil to stop\n\n\n")

function main()
    local M = {
        FIRMWARE_ACCEPT_TIMEOUT = 20000,
        UPDATE_NEW_FILE = "update.img",
        UPDATE_TMP_FILE = "update.img.tmp",
        UPDATE_OLD_FILE = "update.old",
        PROGRAM = "init2.lua",
        DATAFILES_JSON = "datafiles.json"
    }

    M.log = function(level, f, a)
        for i = 1, a.n do
            if a[i] == nil then
                a[i] = "<nil>"
            end
        end
        local st = string.format("[ " .. level .. " ] (boot) " .. f, unpack(a))
        print(st)
        return st
    end

    M.log_info = function(f, ...)
        local a = {n = select("#", ...), ...}
        return M.log("INFO", f, a)
    end

    M.log_error = function(f, ...)
        local a = {n = select("#", ...), ...}
        return M.log("ERROR", f, a)
    end

    M.readJSON = function(fileName)
        local f = file.open(fileName, "r")
        if not f then
            return nil
        end
        local data = ""
        while true do
            local chunk = f:read()
            if chunk ~= nil then
                data = data .. chunk
            else
                break
            end
        end
        f:close()
        local ok, obj = pcall(sjson.decode, data)
        if not ok then
            obj = nil
        end
        return obj
    end

    M.restart = function()
        M.log_info("Restarting in 5 seconds ...")
        tmr.create():alarm(
            5000,
            tmr.ALARM_SINGLE,
            function()
                node.restart()
            end
        )
    end

    M.unpackImage = function(filename)
        M.log_info("Unpacking %s...", filename)
        local f = file.open(filename, "r")
        if f == nil then
            return nil, "Error opening " .. filename .. " firmware file."
        end

        local totalFiles = nil
        --skip other headers. TODO: check these headers for validity
        repeat
            line = f:readline()
            if line ~= nil then
                if totalFiles == nil then
                    totalFiles = tonumber(string.match(line, "Total files:%s*(%d*)\n"))
                end
            end
        until (line == "\n" or line == nil)
        if line == nil then
            return nil, "Cannot find image file body"
        end
        if totalFiles == nil then
            return nil, "Cannot find Total Files header in firmware image"
        end
        M.log_info("unpacking %d files...", totalFiles)
        local fileList = {}
        while totalFiles > 0 do
            local targetFile = f:readline()
            if targetFile == nil then
                return nil, "cannot read targetFile name"
            end
            targetFile = string.match(targetFile, "(.+)\n")
            if targetFile == nil then
                return nil, "Cannot parse targetFile"
            end
            table.insert(fileList, targetFile)
            local size = f:readline()
            if size == nil then
                return nil, "cannot read file size"
            end
            size = string.match(size, "([0-9]+)\n")
            size = tonumber(size)
            if size == nil then
                return nil, "cannot parse file size"
            end
            M.log_info("unpacking %s. Size: %d", targetFile, size)
            local data
            local len
            local tf = file.open(targetFile, "w+")
            if tf == nil then
                return nil, "Error opening targetFile " .. targetFile .. " for writing"
            end
            repeat
                data = f:read(math.min(size, 1024))
                if data == nil then
                    len = 0
                else
                    len = data:len()
                    if tf:write(data) == nil then
                        return nil, "Error writing to targetFile " .. targetFile
                    end
                end
                size = size - len
            until len == 0 or size == 0
            tf:close()
            if size > 0 then
                return nil, string.format(
                    "Firmware file is corrupt, went past end of file unpacking %s (size=%d)",
                    targetFile,
                    size
                )
            end
            totalFiles = totalFiles - 1
        end
        f:close()
        return fileList, nil
    end

    M.cleanup = function(fileList)
        local datafiles = M.readJSON(M.DATAFILES_JSON) or {}
        local list = file.list()
        for _, name in ipairs(datafiles) do
            list[name] = nil
        end
        for _, name in ipairs(fileList) do
            list[name] = nil
        end
        list[M.UPDATE_NEW_FILE] = nil
        list[M.UPDATE_OLD_FILE] = nil
        list[M.UPDATE_TMP_FILE] = nil
        list["init.lua"] = nil
        for name, _ in pairs(list) do
            M.log_info("Removing %s", name)
            file.remove(name)
        end
    end

    M.restorePreviousVersion = function()
        M.log_info("Attempting to restore previous firmware version...")
        fileList, err = M.unpackImage(M.UPDATE_OLD_FILE)
        if err ~= nil then
            M.log_error("Error restoring previous version. Halt.")
            return
        else
            M.cleanup(fileList)
        end
        M.log_info("Restarting after failed update and restoring previous version")
        M.restart()
    end

    M.start = function()
        if file.exists(M.UPDATE_TMP_FILE) then
            M.log_info("Starting new firmware for the first time...")
            M.log_info("Set __FIRMWARE_ACCEPT to true within %d seconds to accept it", M.FIRMWARE_ACCEPT_TIMEOUT / 1000)

            __FIRMWARE_ACCEPT = false
            tmr.create():alarm(
                M.FIRMWARE_ACCEPT_TIMEOUT,
                tmr.ALARM_SINGLE,
                function()
                    if not __FIRMWARE_ACCEPT then
                        M.log_error("New firmware was not accepted timely")
                        file.remove(M.UPDATE_TMP_FILE)
                        M.restorePreviousVersion()
                        return
                    else
                        M.log_info("New firmware was accepted")
                        file.remove(M.UPDATE_OLD_FILE)
                        file.rename(M.UPDATE_TMP_FILE, M.UPDATE_OLD_FILE)
                        __FIRMWARE_ACCEPT = nil
                    end
                end
            )
        else
            if file.exists(M.UPDATE_NEW_FILE) then
                file.remove(M.UPDATE_TMP_FILE)
                file.rename(M.UPDATE_NEW_FILE, M.UPDATE_TMP_FILE)
                local fileList, err = M.unpackImage(M.UPDATE_TMP_FILE)
                if err ~= nil then
                    file.remove(M.UPDATE_TMP_FILE)
                    M.log_error("Error unpacking update file: %s", err)
                    M.restorePreviousVersion()
                    return
                end
                M.cleanup(fileList)
                M.log_info("new firmware was unpacked successfully. Restarting...")
                M.restart()
                return
            end
        end

        if file.exists(M.PROGRAM) then
            M.log_info("Bootloader finished. Launching application...")
            dofile(M.PROGRAM)
        else
            M.log_error("Cannot find %s. Halt.", M.PROGRAM)
        end
    end

    M.start()
end

tmr.create():alarm(
    3000,
    tmr.ALARM_SINGLE,
    function()
        if main ~= nil then
            main()
            main = nil
        end
    end
)


`
