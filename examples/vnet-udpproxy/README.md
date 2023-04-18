# vnet-udpproxy

This example demonstrates how VNet can be used to communicate with non-VNet addresses using UDPProxy.

In this example we listen map the VNet Address `10.0.0.11` to a real address of our choice. We then
send to our real address from three different VNet addresses.

If you pass `-address 192.168.1.3:8000` the traffic will be the following

```
   vnet(10.0.0.11:5787) => proxy => 192.168.1.3:8000
   vnet(10.0.0.11:5788) => proxy => 192.168.1.3:8000
   vnet(10.0.0.11:5789) => proxy => 192.168.1.3:8000
```

## Running
```
go run main.go -address 192.168.1.3:8000
```

You should see the following in tcpdump
```
sean@SeanLaptop:~/go/src/github.com/pion/transport/examples$ sudo tcpdump -i any udp and port 8000
tcpdump: data link type LINUX_SLL2
tcpdump: verbose output suppressed, use -v[v]... for full protocol decode
listening on any, link-type LINUX_SLL2 (Linux cooked v2), snapshot length 262144 bytes
13:21:18.239943 lo    In  IP 192.168.1.7.40574 > 192.168.1.7.8000: UDP, length 5
13:21:18.240105 lo    In  IP 192.168.1.7.40647 > 192.168.1.7.8000: UDP, length 5
13:21:18.240304 lo    In  IP 192.168.1.7.57744 > 192.168.1.7.8000: UDP, length 5
```
