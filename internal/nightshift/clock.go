package nightshift

import "time"

// Now is the only clock read used for an omitted seed and for session start.
var Now = time.Now
