package main

import "testing"

func TestDbInsert(t *testing.T) {
	Init()
	if err := Insert("testid", "testvalue1"); err != nil {
		t.Fatal(err)
	}
	if err := Insert("testid", "testvalue2"); err != nil {
		t.Fatal(err)
	}

	if err := dbDelete("testid"); err != nil {
		t.Fatal(err)
	}

	t.Log("Success")
}
