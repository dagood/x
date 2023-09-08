module example.org/basiccryptouse

go 1.21

require golang.org/x/crypto v0.13.0

require (
	golang.org/x/net v0.15.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/text v0.13.0 // indirect
)

// Add the replace explicitly for this example so a normal Microsoft Go toolset
// can be used even without the new x/crypto swap experiment (although using the
// experiment will still replace this replacement with its own). Make sure to
// "go git-patch apply" first in "../../crypto".
replace golang.org/x/crypto => ../../crypto/upstream
