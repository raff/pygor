# test exceptions
try:
    x = 1/0
except Exception:
    x = None
else:
    x = False
finally:
    print(x)

