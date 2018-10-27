package runtime

import "testing"

func TestAssert(t *testing.T) {
	Assert(true, "this should be true")
}

func TestContainsString(t *testing.T) {
	bag := "the quick brown fox"

	if !Contains(bag, "fox") {
		t.Error(bag, "should contain fox")
	}

	if Contains(bag, "dog") {
		t.Error(bag, "should not contain dog")
	}
}

func TestContainsList(t *testing.T) {
	bag := List{"one", "two", "three"}

	if !Contains(bag, "one") {
		t.Error(bag, "should contain one")
	}

	if Contains(bag, "four") {
		t.Error(bag, "should not contain four")
	}
}

func TestContainsDict(t *testing.T) {
	bag := Dict{"one": 1, "two": 2, "three": 3}

	if !Contains(bag, "one") {
		t.Error(bag, "should contain one")
	}

	if Contains(bag, "four") {
		t.Error(bag, "should not contain four")
	}
}

func TestContainsFloat(t *testing.T) {
	bag := 3.14

	// can't really check if a float contain something
	if Contains(bag, 3.14) {
		t.Error(bag, "is not a container")
	}
}

func TestIsSpace(t *testing.T) {
	if !IsSpace(" \t\r\n") {
		t.Error("all spaces")
	}

	if IsSpace(" . ") {
		t.Error("not all spaces")
	}

	if IsSpace("") {
		t.Error("empty string is not a space")
	}
}

func TestIsAlpha(t *testing.T) {
	if !IsAlpha("abcdEFGH") {
		t.Error("all alpha")
	}

	if IsAlpha("abcdEFGH1") {
		t.Error("digits are not alpha")
	}

	if IsAlpha("abcd EFGH1") {
		t.Error("spaces are not alpha")
	}

	if IsAlpha("") {
		t.Error("empty string is not alpha")
	}
}

func TestIsDigit(t *testing.T) {
	if !IsDigit("1234567890") {
		t.Error("all digits")
	}

	if IsDigit("123456789O") {
		t.Error("alpha are not digits")
	}

	if IsDigit("1234 5678") {
		t.Error("spaces are not digits")
	}

	if IsDigit("") {
		t.Error("empty string is not digit")
	}
}

func TestIsUpper(t *testing.T) {
	if !IsUpper("ABCDEFGH") {
		t.Error("all upper")
	}

	if !IsUpper("ABCD EFGH") {
		t.Error("all upper and spaces")
	}

	if IsUpper("ABCDefgh") {
		t.Error("lower are not upper")
	}

	if IsUpper("    ") {
		t.Error("spaces are not upper")
	}

	if IsUpper("") {
		t.Error("empty string is not upper")
	}
}

func TestIsLower(t *testing.T) {
	if !IsLower("abcdefgh") {
		t.Error("all lower")
	}

	if !IsLower("abcd efgh") {
		t.Error("all lower and spaces")
	}

	if IsLower("abcdEFGH") {
		t.Error("uppper are not lower")
	}

	if IsLower("    ") {
		t.Error("spaces are not lower")
	}

	if IsLower("") {
		t.Error("empty string is not lower")
	}
}
