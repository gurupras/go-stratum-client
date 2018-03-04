# go-stratum-client
Stratum client implemented in Go (WIP)

This is an extremely primitive stratum client implemented in Golang. 
Most of the code was implemented by going through the sources of cpuminer-multi and xmrig.

## What can it do?
  - Connect to pool
  - Authenticate
  - Receive jobs
  - Submit results
  - Keepalive

## What's pending
  - Reconnect on error
  - Whatever else the stratum protocol specifies
  
## Can I help?
Yes! Please send me PRs that can fix bugs, clean up the implementation and improve it.

### I'll just fork and improve it myself
I _will_ find it and I _will_ merge it.
