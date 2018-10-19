# This that could be done

- Convert "string".join(list) to strings.Join(list, "string")

- isinstance(v, (t1, t2, t3))

- int(s, base) - but int() can also be int(string, base=10) or int(number)

- for x,y,z in list_of_tuples: this could be converted to for _t := range _list_of tuples { x,y,z = _t; ... }

- Do something with 'yield'. Generator can probably be implemented as list/dict comprehension generators 
    (a goroutine writing to a channel). So if a function body contains a "yield" it could be wrapped in
    an anonymous function, called as a goroutine and the real function should return a channel.
