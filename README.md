# pygor
Python to Go Regurgitator

A Python 3 to Go transpiler, with many things to be desired.

## installation

    go get -v -u github.com/raff/pygor
    
## usage

    go run pygor.go python_code.py
    
## tests

    for f in tests/*.py
    do
      go run pygor.go $f
    done
