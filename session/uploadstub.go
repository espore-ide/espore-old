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

    L.upload = function(fname, size)
        local remaining = size
        local f = file.open(fname, "w+")
        local h = crypto.new_hash("sha1")
        local nextChunk

        local function writer(data)
            f:write(data)
            h:update(data)
            remaining = remaining - #data
            nextChunk()
        end

        nextChunk = function()
            print(remaining)
            if remaining <= 0 then
                f:close()
                uart.on("data")
                local hash = encoder.toHex(h:finalize())
                print(hash)
                return
            end

            local chunkSize = remaining
            if chunkSize > 255 then
                chunkSize = 255
            end
            uart.on("data", chunkSize, writer, 0)
        end

        nextChunk()
        print("BEGIN")
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
        for k in pairs(list) do keys[#keys+1] = k end
        table.sort(keys)
        for _, key in ipairs(keys) do
            print(key .. "\t" .. list[key])
        end
    end
    __espore = L
    L.start()
end)()

`
