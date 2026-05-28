package extract

import "fmt"

func stableID(file, q string, line int) string { return fmt.Sprintf("%s::%s::%d", file, q, line) }
func strPtr(s string) *string                  { return &s }
func intPtr(i int) *int                        { return &i }
