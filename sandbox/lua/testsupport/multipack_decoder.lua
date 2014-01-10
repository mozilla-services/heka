function process_message ()
    output("message one")
    inject_message()
    output("message two")
    inject_message()

    local hm = {Payload = "message three"}
    inject_message(hm)
    return 0
end
