print("\n\n\nEspore bootloader will launch in 3 seconds.")
print("Set boot to nil to stop\n\n\n")

function runMain()
    runMain = nil
    local ok, mainFunc = pcall(require, "main")
    if not ok then print("Error loading main module: ", modFunc) end
    if type(mainFunc) == "function" then
        local ok, err = pcall(mainFunc)
        if not ok then print("Error invoking main function: ", err) end
    end
end

function boot()
    local M = {
        FIRMWARE_ACCEPT_TIMEOUT = 60000,
        UPDATE_NEW_FILE = "update.img",
        UPDATE_1ST_FILE = "update.img.1st",
        UPDATE_FAIL_FILE = "update.img.fail",
        UPDATE_OLD_FILE = "update.old",
        LFS_NEW_FILE = "lfs.img",
        LFS_TMP_FILE = "lfs.img.tmp",
        DATAFILES_JSON = "datafiles.json"
    }

    M.log = function(level, f, a)
        for i = 1, a.n do if a[i] == nil then a[i] = "<nil>" end end
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
        if not f then return nil end
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
        if not ok then obj = nil end
        return obj
    end

    M.restart = function()
        M.log_info("Restarting in 5 seconds. Set 'restart' to false to stop ...")
        restart = true
        tmr.create():alarm(5000, tmr.ALARM_SINGLE, function()
            if restart then
                node.restart()
            else
                M.log_info("restart cancelled.")
            end
        end)
    end

    M.unpackImage = function(filename)
        M.log_info("Unpacking %s...", filename)
        local f = file.open(filename, "r")
        if f == nil then
            return nil, "Error opening " .. filename .. " firmware file."
        end

        local totalFiles = nil
        -- skip other headers. TODO: check these headers for validity
        repeat
            line = f:readline()
            if line ~= nil then
                if totalFiles == nil then
                    totalFiles = tonumber(
                                     string.match(line, "Total files:%s*(%d*)\n"))
                end
            end
        until (line == "\n" or line == nil)
        if line == nil then return nil, "Cannot find image file body" end
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
            if size == nil then return nil, "cannot read file size" end
            size = string.match(size, "([0-9]+)\n")
            size = tonumber(size)
            if size == nil then return nil, "cannot parse file size" end
            M.log_info("unpacking %s. Size: %d", targetFile, size)
            local data
            local len
            local tf = file.open(targetFile, "w+")
            if tf == nil then
                return nil, "Error opening targetFile " .. targetFile ..
                           " for writing"
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
                           targetFile, size)
            end
            totalFiles = totalFiles - 1
        end
        f:close()
        return fileList, nil
    end

    M.cleanup = function(fileList)
        local datafiles = M.readJSON(M.DATAFILES_JSON) or {}
        local list = file.list()
        for _, name in ipairs(datafiles) do list[name] = nil end
        for _, name in ipairs(fileList) do list[name] = nil end
        list[M.UPDATE_NEW_FILE] = nil
        list[M.UPDATE_OLD_FILE] = nil
        list[M.UPDATE_1ST_FILE] = nil
        list[M.UPDATE_FAIL_FILE] = nil
        list["init.lua"] = nil
        for name, _ in pairs(list) do
            M.log_info("Removing %s", name)
            file.remove(name)
        end
    end

    M.flashLFS = function()
        if node.flashindex and file.exists(M.LFS_NEW_FILE) then
            print("Found LFS image. Flashing ...")
            file.remove(M.LFS_TMP_FILE)
            file.rename(M.LFS_NEW_FILE, M.LFS_TMP_FILE)
            collectgarbage()
            local err = node.flashreload(M.LFS_TMP_FILE)
            print("Error flashing LFS image: " .. err)
            return err
        end
    end

    M.restorePreviousVersion =
        function() -- TODO: restore previous version should also restore updater-etag!
            M.log_info("Attempting to restore previous firmware version...")
            file.remove(M.UPDATE_1ST_FILE)
            file.remove(M.UPDATE_FAIL_FILE)
            file.remove(M.UPDATE_NEW_FILE)
            fileList, err = M.unpackImage(M.UPDATE_OLD_FILE)
            if err ~= nil then
                M.log_error("Error restoring previous version. Halt.")
                return
            else
                M.cleanup(fileList)
            end
            M.log_info(
                "Restarting after failed update and restoring previous version")
            M.flashLFS()
            M.restart()
        end

    M.start = function()
        if file.exists(M.UPDATE_FAIL_FILE) then
            M.log_error(
                "Update failed to be accepted. Rolling back to previous version")
            M.restorePreviousVersion()
            return
        else
            if file.exists(M.UPDATE_1ST_FILE) then
                file.remove(M.LFS_TMP_FILE)
                file.remove(M.UPDATE_FAIL_FILE)
                file.rename(M.UPDATE_1ST_FILE, M.UPDATE_FAIL_FILE)
                M.log_info("Starting new firmware for the first time...")
                M.log_info(
                    "Call __acceptFirmware() to accept it before a reboot.")

                __acceptFirmware = loadstring(
                                       string.format(
                                           [[
                    file.remove("%s")
                    file.rename("%s", "%s")
                    print("[boot] New firmware was accepted")
                    __acceptFirmware = nil
                ]], M.UPDATE_OLD_FILE, M.UPDATE_FAIL_FILE, M.UPDATE_OLD_FILE))
            else
                if file.exists(M.UPDATE_NEW_FILE) then
                    file.remove(M.UPDATE_1ST_FILE)
                    file.rename(M.UPDATE_NEW_FILE, M.UPDATE_1ST_FILE)
                    local fileList, err = M.unpackImage(M.UPDATE_1ST_FILE)
                    if err ~= nil then
                        M.log_error("Error unpacking update file: %s", err)
                        M.restorePreviousVersion()
                        return
                    end
                    M.cleanup(fileList)
                    if M.flashLFS() ~= nil then
                        M.log_error("Error flashing LFS: %s", err)
                        M.restorePreviousVersion()
                        return
                    end
                    M.log_info(
                        "new firmware was unpacked successfully. Restarting...")
                    M.restart()
                    return
                end
            end
        end

        if node.flashindex then
            local ok, err = pcall(node.flashindex("__lfsinit"))
            if not ok then
                M.log_error("Error loading LFS modules: %s", err)
            else
                M.log_info("LFS initialized")
            end
        end
        tmr.create():alarm(1, tmr.ALARM_SINGLE, runMain)
    end
    M.start()
end

tmr.create():alarm(3000, tmr.ALARM_SINGLE, function()
    if boot ~= nil then
        boot()
        boot = nil
        collectgarbage()
    end
end)
