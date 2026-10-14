// Package middleware provides Grove's built-in HTTP middleware:
// recovery, request logging, CORS, compression, timeouts, and rate
// limiting. DefaultChain is the safe global stack; ProductionChain adds
// compression. CORS, RateLimit, and Timeout are opt-in per app/route —
// see their constructors for the trade-offs.
package middleware
