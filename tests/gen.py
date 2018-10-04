# test generators
gen = ("%s:%s" % (k, v) for k,v in {"a":1, "b":2, "c":3, "d":4}.items() if v % 2 == 0)

for v in gen:
    print(v)

