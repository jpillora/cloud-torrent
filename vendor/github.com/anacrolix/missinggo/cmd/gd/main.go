package main

import (
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"strings"
)

func escape(encoding string) {
	switch {
	case strings.HasPrefix("query", encoding):
		b, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Stdout.Write([]byte(url.QueryEscape(string(b))))
	case strings.HasPrefix("hex", encoding):
		b, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Stdout.Write([]byte(hex.EncodeToString(b)))
	default:
		fmt.Fprintf(os.Stderr, "unknown escape encoding: %q\n", encoding)
		os.Exit(2)
	}
}

func unescape(encoding string) {
	switch {
	case strings.HasPrefix("query", encoding):
		b, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		s, err := url.QueryUnescape(string(b))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Stdout.Write([]byte(s))
	case strings.HasPrefix("b32", encoding):
		d := base32.NewDecoder(base32.StdEncoding, os.Stdin)
		io.Copy(os.Stdout, d)
	default:
		fmt.Fprintf(os.Stderr, "unknown unescape encoding: %q\n", encoding)
	}
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "expected two arguments: <mode> <encoding>: got %d\n", len(os.Args)-1)
		os.Exit(2)
	}
	mode := os.Args[1]
	switch {
	case strings.HasPrefix("escape", mode):
		escape(os.Args[2])
	case strings.HasPrefix("unescape", mode) || strings.HasPrefix("decode", mode):
		unescape(os.Args[2])
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %q\n", mode)
		os.Exit(2)
	}
}
