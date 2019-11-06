package session

const upbin = `

(function()
    local L = {}
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
        local rprint = print
        local printbuf = {}
        local timer = tmr.create()
        timer:register(
            500,
            tmr.ALARM_AUTO,
            function()
                rprint(size - remaining)
            end
        )
        print = function(txt)
            if #printbuf < 50 then
                table.insert(printbuf, txt)
            end
        end

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
            if remaining <= 0 then
                f:close()
                local hash = encoder.toHex(h:finalize())
                rprint(hash)
                uart.on("data")
                print = rprint
                timer:stop()
                timer:unregister()
                for _, txt in ipairs(printbuf) do
                    print(txt)
                end
                return
            end

            local chunkSize = remaining
            if chunkSize > 128 then
                chunkSize = 128
            end
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

    L.ls = function()
        local list = file.list()
        local keys = {}
        for k in pairs(list) do
            keys[#keys + 1] = k
        end
        table.sort(keys)
        for _, key in ipairs(keys) do
            print(key .. "\t" .. list[key])
        end
    end
    __espore = L
    L.start()
end)()


`
