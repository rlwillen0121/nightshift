package nightshift

import "time"

// Now is the only clock read used for an omitted seed and for session start.
var Now = time.Now

// sleep paces write-time effects. Tests replace it so they never wait.
var sleep = time.Sleep
