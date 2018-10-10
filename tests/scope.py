# test variables scope

a = 42

def f(a, b):
    a = b + 3 / a
    return a

a = f(a, 12)
b = a + 1
