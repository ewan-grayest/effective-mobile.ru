package main

import "testing"

func TestParseDate(t *testing.T) {
	good, err := parseDate("07-2025")
	if err != nil {
		t.Fatal(err)
	}

	if good.Year() != 2025 {
		t.Fatal("wrong year")
	}

	if int(good.Month()) != 7 {
		t.Fatal("wrong month")
	}

	_, err = parseDate("2025-07")
	if err == nil {
		t.Fatal("bad date should give error")
	}
}

func TestFormatDate(t *testing.T) {
	date, err := parseDate("11-2026")
	if err != nil {
		t.Fatal(err)
	}

	text := formatDate(date)
	if text != "11-2026" {
		t.Fatal("bad formatted date")
	}
}

func TestUUID(t *testing.T) {
	id := makeNewID()
	if !isUUID(id) {
		t.Fatal("new id is not uuid")
	}

	if isUUID("hello") {
		t.Fatal("hello is not uuid")
	}
}
