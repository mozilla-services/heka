function process_message ()
    inject_payload("txt", "", "message one")
    inject_payload("txt", "", "message two")

    local hm = {Payload = "message three"}
    inject_message(hm)
    return 0
end
