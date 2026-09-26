// Command catalogen writes internal/platform/events/catalog.gen.go; run via `go generate`.
package main

import (
	"log"
	"os"

	"pdpa-platform/internal/platform/events/internal/catalogen"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: catalogen <events.yaml> <out.go>")
	}
	src, err := catalogen.Render(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(os.Args[2], src, 0o644); err != nil {
		log.Fatal(err)
	}
}
