package domain

var axisNames = [6]string{"+X", "-X", "+Y", "-Y", "+Z", "-Z"}
var axes = [6]Vec{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}

type Lobes [6]Vec
