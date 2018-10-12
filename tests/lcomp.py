# test list comprehension

print([x.upper() for x in ["one", "two", "three", "four", "five", "six"]])

print([x.upper() for x in ["one", "two", "three", "four", "five", "six"] if len(x) <= 4])

print([x for x in range(10)])
