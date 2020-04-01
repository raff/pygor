# test class definition

class test(object):
    cvar1 = 12
    cvar2 = "hello"

    def __init__(self, n):
        self.n = n

    def __str__(self):
        return "test object"

    def printn(self, x):
        print(self.n * x)

    #
    # this is currently not supported
    #
    #class nested(object):
    #    pass
        
