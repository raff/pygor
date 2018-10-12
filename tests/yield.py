# test yield

def gen(n):
    for i in range(n):
        yield i

for x in gen(5):
    print(x)
