// Package app wires the relayhat runtime together.
//
// It initializes configured relays, sets up authenticated HTTP routes and
// middleware, starts the HTTPS server, and coordinates graceful shutdown and
// SIGHUP-based reloads.
package app
