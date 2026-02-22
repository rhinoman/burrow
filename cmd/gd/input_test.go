package main

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestReadMultiLine(t *testing.T) {
	// Simulate pasted multi-line input: all data is buffered at once.
	input := "line one\nline two\nline three\n"
	reader := bufio.NewReader(strings.NewReader(input))

	result, err := readMultiLine(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "line one\nline two\nline three"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestReadMultiLineSingleLine(t *testing.T) {
	input := "hello world\n"
	reader := bufio.NewReader(strings.NewReader(input))

	result, err := readMultiLine(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("got %q, want %q", result, "hello world")
	}
}

func TestReadMultiLineEmptyLines(t *testing.T) {
	// Blank lines in a paste should be skipped.
	input := "first\n\n\nsecond\n"
	reader := bufio.NewReader(strings.NewReader(input))

	result, err := readMultiLine(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "first\nsecond"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestReadMultiLineEOF(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(""))

	_, err := readMultiLine(reader)
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestReadConfirm(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("  y  \n"))

	result := readConfirm(reader)
	if result != "y" {
		t.Errorf("got %q, want %q", result, "y")
	}
}
