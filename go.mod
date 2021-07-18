module github.com/raphaelgavache/go-tuf

go 1.14

require (
	github.com/dustin/go-humanize v1.0.0
	github.com/flynn/go-docopt v0.0.0-20140912013429-f6dd2ebbb31e
	github.com/stretchr/testify v1.7.0
	github.com/syndtr/goleveldb v1.0.0
	github.com/tent/canonical-json-go v0.0.0-20130607151641-96e4ba3a7613
	github.com/theupdateframework/go-tuf v0.0.0-local
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f
)

replace github.com/theupdateframework/go-tuf => ./
