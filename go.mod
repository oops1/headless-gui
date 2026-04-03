module github.com/oops1/headless-gui/v3

go 1.22.0

toolchain go1.24.1

require (
	github.com/oops1/headless-gui/v3/window v0.0.0
	golang.org/x/image v0.15.0
)

require (
	github.com/ebitengine/purego v0.7.1 // indirect
	golang.org/x/sys v0.26.0 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace github.com/oops1/headless-gui/v3/window => ./window
