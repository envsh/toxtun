### 如何避免使用锁，并且能够使用上goroutine

使用ctrl loop + channel(with cache)

阻塞处理的goroutine把数据通过通过channel发送到channel。

ctrl loop 单线程方式处理更高级的逻辑。

### main.main中return 与 os.Exit 的区别

return 会执行 defer 的代码。

os.Exit 不会执行 defer 的代码。

### 不同线路中选择快速线路的几点考虑
* 发送速度
* 接收速度
* ping响应时间 RTT
* loss rates
* 实时数据的ACK

* TCP Throughput Testing: https://tools.ietf.org/html/rfc6349
* Softethervpn's Parallel Transmission Mechanism of Multiple Tunnels
* transfer data between 2 systems as fast as possible over multiple TCP paths: https://github.com/facebook/wdt
* https://fishi.devtail.io/weblog/2015/04/12/measuring-bandwidth-and-round-trip-time-tcp-connection-inside-application-layer/

自动选择线路功能实现大致完成，测试效果不太好，有些因素导致选择的线路切换频繁，速度反而提不上去。
还需要考虑这种方式的优化的可能性与优化方法。
这个问题归为MPTCP schedule范畴。https://tools.ietf.org/html/rfc6356

### 优化1：在kcp握手的阶段，数据一同发送到服务器端。 (首次连接的RTT)

