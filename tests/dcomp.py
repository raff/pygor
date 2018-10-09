# test list comprehension

print({x.upper():len(x) for x in ["one", "two", "three", "four", "five", "six"]})

print({x.upper():len(x) for x in ["one", "two", "three", "four", "five", "six"] if len(x) <= 4})
