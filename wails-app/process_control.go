package main

import "errors"

// errProcessGone marks a pause or resume aimed at a process that already ended
// or is on its way out. The queue moves between episodes on its own, so the
// window between "the button was pressed" and "FFmpeg exited" is normal — it is
// never a failure worth showing the user.
var errProcessGone = errors.New("tiến trình đã kết thúc")
