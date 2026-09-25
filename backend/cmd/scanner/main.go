// Command scanner will crawl tenant websites to classify cookies against consent.cookie_catalog
// (docs/modules/CON.md, feature CON-02+). It runs in its own node pool with an egress proxy
// (docs/architecture/deployment.md) since it's the one component that reaches the public internet.
// Not implemented yet — cookie scanning is P1, after the P0 foundation this binary is scaffolded
// for. Wire it up when CON-02 is picked up via /implement.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "scanner: not implemented yet — see docs/modules/CON.md (CON-02)")
	os.Exit(1)
}
