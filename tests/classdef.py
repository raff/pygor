# test class definition

class test(object):
    def __init__(self, n):
        self.n = n

    def __str__(self):
        return "test object"

    def printn(self, x):
        print(self.n * x)
        
