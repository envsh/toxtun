### 如何避免使用锁，并且能够使用上goroutine

使用ctrl loop + channel(with cache)

阻塞处理的goroutine把数据通过通过channel发送到channel。

ctrl loop 单线程方式处理更高级的逻辑。

### main.main中return 与 os.Exit 的区别

return 会执行 defer 的代码。

os.Exit 不会执行 defer 的代码。

