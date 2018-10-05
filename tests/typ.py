def greeting(name: str) -> str:
    return 'Hello ' + name

def noreturn(n: int, l: list) -> None:
    print(n, l)

def returns(b: bool, d: dict) -> (bool, dict):
    return b, d
