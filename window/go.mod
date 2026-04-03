module github.com/oops1/headless-gui/v3/window

go 1.22.0

require (
	github.com/ebitengine/purego v0.7.1
	github.com/oops1/headless-gui/v3 v3.0.0
	golang.org/x/sys v0.26.0
)

require (
	golang.org/x/image v0.15.0 // indirect
	golang.org/x/text v0.14.0  // indirect
)

replace github.com/oops1/headless-gui/v3 => ../
