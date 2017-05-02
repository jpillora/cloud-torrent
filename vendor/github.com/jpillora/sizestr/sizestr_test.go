package sizestr

import (
	"flag"
	"testing"
)

func TestToString1(t *testing.T) {
	str := ToString(231938)
	if str != "232KB" {
		t.Error(str)
	}
}

func TestToString1b(t *testing.T) {
	str := ToStringSig(231938, 4)
	if str != "231.9KB" {
		t.Error(str)
	}
}

func TestToString2(t *testing.T) {
	str := ToString(1000000000)
	if str != "1GB" {
		t.Error(str)
	}
}

func TestToString3(t *testing.T) {
	str := ToString(999999)
	if str != "1MB" {
		t.Error(str)
	}
}

func TestToString4(t *testing.T) {
	str := ToString(1)
	if str != "1B" {
		t.Error(str)
	}
}

func TestToString5(t *testing.T) {
	str := ToString(1200000000)
	if str != "1.2GB" {
		t.Error(str)
	}
}

func TestToString6(t *testing.T) {
	str := ToStringSig(1200000000, 1)
	if str != "1GB" {
		t.Error(str)
	}
}

func TestToString7(t *testing.T) {
	str := ToStringSig(1234567890, 5)
	if str != "1.2346GB" {
		t.Error(str)
	}
}

func TestToString8(t *testing.T) {
	str := ToStringSigBytesPerKB(231938, 4, 1024)
	if str != "226.5KB" {
		t.Error(str)
	}
}

func TestParse1(t *testing.T) {
	b, err := Parse("232KB")
	if err != nil {
		t.Fatal(err)
	}
	if b != 232000 {
		t.Error(b)
	}
}

func TestParse2(t *testing.T) {
	b, err := Parse("4GB")
	if err != nil {
		t.Fatal(err)
	}
	if b != 4*1000*1000*1000 {
		t.Error(b)
	}
}

func TestParse3(t *testing.T) {
	b, err := Parse("7b")
	if err != nil {
		t.Fatal(err)
	}
	if b != 7 {
		t.Error(b)
	}
}

func TestParse4(t *testing.T) {
	b, err := Parse("1kib")
	if err != nil {
		t.Fatal(err)
	}
	if b != 1024 {
		t.Error(b)
	}
}

func TestParse5(t *testing.T) {
	b, err := Parse("2GiB")
	if err != nil {
		t.Fatal(err)
	}
	if b != 2*1024*1024*1024 {
		t.Error(b)
	}
}

func TestParse6(t *testing.T) {
	b, err := Parse("5pb")
	if err != nil {
		t.Fatal(err)
	}
	if b != 5*1000*1000*1000*1000*1000 {
		t.Error(b)
	}
}

func TestFlags1(t *testing.T) {
	b := Bytes(MustParse("25kb"))
	f := flag.NewFlagSet("test", flag.ContinueOnError)
	f.Var(&b, "size", "give me a size")
	err := f.Parse([]string{"-size", "123kb"})
	if err != nil {
		t.Fatal(err)
	}
	if int64(b) != 123*1000 {
		t.Error(b)
	}
}
