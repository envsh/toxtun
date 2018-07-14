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
* ping响应时间

* TCP Throughput Testing: https://tools.ietf.org/html/rfc6349
* Softethervpn's Parallel Transmission Mechanism of Multiple Tunnels
* transfer data between 2 systems as fast as possible over multiple TCP paths: https://github.com/facebook/wdt

