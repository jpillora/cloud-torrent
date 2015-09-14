package sizestr

import "testing"

func Test1(t *testing.T) {
	str := ToString(231938)
	if str != "232KB" {
		t.Error(str)
	}
}

func Test1b(t *testing.T) {
	str := ToStringSig(231938, 4)
	if str != "231.9KB" {
		t.Error(str)
	}
}

func Test2(t *testing.T) {
	str := ToString(1000000000)
	if str != "1GB" {
		t.Error(str)
	}
}

func Test3(t *testing.T) {
	str := ToString(999999)
	if str != "1MB" {
		t.Error(str)
	}
}

func Test4(t *testing.T) {
	str := ToString(1)
	if str != "1B" {
		t.Error(str)
	}
}

func Test5(t *testing.T) {
	str := ToString(1200000000)
	if str != "1.2GB" {
		t.Error(str)
	}
}

func Test6(t *testing.T) {
	str := ToStringSig(1200000000, 1)
	if str != "1GB" {
		t.Error(str)
	}
}

func Test7(t *testing.T) {
	str := ToStringSig(1234567890, 5)
	if str != "1.2346GB" {
		t.Error(str)
	}
}

func Test8(t *testing.T) {
	str := ToStringSigScale(231938, 4, 1024)
	if str != "226.5KB" {
		t.Error(str)
	}
}
