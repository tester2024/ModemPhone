package main

import "path/filepath"

func dirOf(p string) string      { return filepath.Dir(p) }
func dirJoin(a ...string) string { return filepath.Join(a...) }
