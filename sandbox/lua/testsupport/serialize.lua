count = 0
rate = 0.12345678
rates = {99.1,98,97, key="val"}
kvp = {a="foo", b="bar", r=rates}
nested = {arg1=1, arg2=2, nested={n1="one",n2="two"}, empty = nil}
_G["key with spaces"] = "kws"
boolean = true
empty = nil
func = function (s) return s end
uuids = {
    {uuid="BD48B609-8922-4E59-A358-C242075CE088", type="test"},
    {uuid="BD48B609-8922-4E59-A358-C242075CE089", type="test1"}
}

large_key = {
    aaaaaaaaaaaaaaaaaaa = {["BD48B609-8922-4E59-A358-C242075CE081"] = 1,
    bbbbbbbbbbbbbbbbbbb = {["BD48B609-8922-4E59-A358-C242075CE082"] = 2,
    ccccccccccccccccccc = {["BD48B609-8922-4E59-A358-C242075CE083"] = 3,
    ddddddddddddddddddd = {["BD48B609-8922-4E59-A358-C242075CE084"] = 4,
    eeeeeeeeeeeeeeeeeee = {["BD48B609-8922-4E59-A358-C242075CE085"] = 5,
    fffffffffffffffffff = {["BD48B609-8922-4E59-A358-C242075CE086"] = 6,
    ggggggggggggggggggg = {["BD48B609-8922-4E59-A358-C242075CE087"] = 7,
    hhhhhhhhhhhhhhhhhhh = {["BD48B609-8922-4E59-A358-C242075CE088"] = 8,
    iiiiiiiiiiiiiiiiiii = {["BD48B609-8922-4E59-A358-C242075CE089"] = 9,}}}}}}}}}
}

cyclea = {type="cycle a"}
cycleb = {type="cycle b"}
cyclea["b"] = cycleb
cycleb["a"] = cyclea

function process_message ()
    return 0
end


function timer_event(ns)
end


