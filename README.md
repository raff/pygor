# gopyr
Python to Go regurgitator

A Python 3 to Go transpiler, with many things to be desired.

## installation

    go get -v -u github.com/raff/gopyr
    
## usage

    go run gopyr.go python_code.py
    
## tests

    for f in tests/*.py
    do
      go run gopyr.go $f
    done
