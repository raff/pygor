# test generators

gen1 = ("%s:%s" % (k, v) for k,v in {"a":1, "b":2, "c":3, "d":4}.items())
gen2 = ("%s:%s" % (k, v) for k,v in {"a":1, "b":2, "c":3, "d":4}.items() if v % 2 == 0)

for v in gen1:
    print(v)

for v in gen2:
    print(v)

