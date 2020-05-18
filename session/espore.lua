(function()
    local L = {}
    local rprint = print
    local printbuf = {}
    local printLock = function()
        print = function(txt)
            if #printbuf < 50 then table.insert(printbuf, txt) end
        end
    end
    local printUnlock = function()
        print = rprint
        for _, txt in ipairs(printbuf) do print(txt) end
        printbuf = {}
    end

    local function is_array(tbl) return tbl[1] ~= nil end
    local function stjson(obj)
        local t = type(obj)
        if t == "table" then
            if is_array(obj) then
                rprint("[")
                for i, v in ipairs(obj) do
                    if i ~= 1 then rprint(",") end
                    stjson(v)
                end
                rprint("]")
            else
                rprint("{")
                local first = false
                for k, v in pairs(obj) do
                    if first then
                        rprint(",")
                    else
                        first = true
                    end
                    rprint('"' .. k .. '":')
                    stjson(v)
                end
                rprint("}")

            end
        else
            if t == "number" or t == "boolean" then
                rprint(obj)
            else
                if t == "string" then
                    rprint('"' .. obj:gsub('"', '\\\"'):gsub("\n", '\\n') .. '"')
                else
                    rprint("null")
                end
            end
        end
    end

    L.callAsync = function(f, timeout)
        local timer
        local called = false
        local callback = function(ret, err)
            if timer ~= nil then
                timer:stop()
                timer:unregister()
                timer = nil
            end
            if not called then
                called = true
                rprint("")
                stjson({ret = ret, err = err})
                printUnlock()
            end
        end
        printLock()
        local ok, ret = pcall(f, callback)
        if not ok then
            callback(nil, ret)
            return
        end
        if not called then
            if timeout == nil then timeout = 3000 end
            timer = tmr.create()
            timer:register(timeout, tmr.ALARM_SINGLE,
                           function() callback(nil, "TIMEOUT") end)
            timer:start()
        end
    end

    L.call = function(f, timeout)
        L.callAsync(function(callback)
            local ok, ret = pcall(f)
            if ok then
                callback(ret, nil)
            else
                callback(nil, ret)
            end
        end, timeout)
    end

    L.echo = function(value)
        local b, d, p, s = uart.getconfig(0)
        uart.setup(0, b, d, p, s, value)
    end

    L.start = function()
        __espore.echo(0)
        print("\nREADY")
    end

    L.finish = function()
        print("\nBYE")
        __espore.echo(1)
        __espore = nil
    end

    L.rename = function(oldname, newname)
        if file.exists(oldname) then
            file.remove(newname)
            file.rename(oldname, newname)
            print("RENAME_OK")
        else
            print("RENAME_FAIL")
        end
    end

    L.upload = function(fname, size)
        local remaining = size
        local f = file.open(fname, "w+")
        local h = crypto.new_hash("sha1")
        local nextChunk
        local timer = tmr.create()
        local timeout
        local cleanup = function()
            f:close()
            uart.on("data")
            timer:stop()
            timer:unregister()
            printUnlock()
        end
        printLock()
        timer:register(500, tmr.ALARM_AUTO, function()
            rprint(size - remaining)
            timeout = timeout + 1
            if timeout > 10 then
                rprint("\n\nTransfer timeout")
                cleanup()
            end
        end)

        local function writer(data)
            f:write(data)
            h:update(data)
            remaining = remaining - #data
            nextChunk()
        end

        nextChunk = function()
            timer:stop()
            timer:start()
            rprint(size - remaining)
            timeout = 0
            if remaining <= 0 then
                local hash = encoder.toHex(h:finalize())
                rprint(hash)
                cleanup()
                return
            end

            local chunkSize = remaining
            if chunkSize > 128 then chunkSize = 128 end
            uart.on("data", chunkSize, writer, 0)
        end

        rprint("\nBEGIN")
        nextChunk()
    end

    L.unload = function(packageName)
        package.loaded[packageName] = nil
        _G[packageName] = nil
    end

    L.unloadAll = function()
        local packages = {}
        for packageName, _ in pairs(package.loaded) do
            packages[#packages] = packageName
        end
        for _, packageName in ipairs(packages) do
            __espore.unload(packageName)
        end
    end

    L.removeFile = function(fileName)
        if file.exists(fileName) then
            file.remove(fileName)
        else
            error("File does not exist")
        end
    end
    __espore = L
    L.start()
end)()

