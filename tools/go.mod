// Build tools, kept out of the app's module so the app gains no dependencies.
module github.com/Perlsowisdom/xegesis/tools

go 1.27.0

require github.com/tc-hib/winres v0.3.1

require (
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
	golang.org/x/image v0.12.0 // indirect
)
