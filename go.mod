module github.com/rubiojr/aemet-go

// opendata.aemet.es uses broken TLS
godebug tlsrsakex=1

go 1.24.3

require github.com/urfave/cli/v3 v3.3.3
