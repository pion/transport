# vnet-nattraversal

A small example that shows basic NAT traversal (UDP hole punching) using
[pion/transport](https://github.com/pion/transport)'s `vnet` package. Everything
runs inside one process, no real network needed.

## What it does

- builds a fake internet with two home networks, each behind its own NAT
- puts one device in each home network
- runs a tiny STUN server on the fake internet (`1.2.3.4:3478`)
- each device asks the STUN server for its public address
- the devices then send UDP packets to each other's public address
- if both print "received", the hole punch worked



## Run it

```sh
go run main.go
```

## Example output

```
Started stun server
device 1 public: 1.2.3.1:40000
device 2 public: 1.2.3.2:40000
1 received: hole-punch-from-B from 1.2.3.2:40000
2 received: hole-punch-from-A from 1.2.3.1:40000
```

The IPs and ports will be different on your machine.

## Things to try

- change `FilteringBehavior` in `natCfg` to `vnet.AddressDependent` or
  `vnet.AddressAndPortDependent` to see how stricter NATs behave
- set `PION_LOG_TRACE=all` before `go run main.go` to see what the routers do

