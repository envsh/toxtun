
### 客户端统计数据

    toxtun stats: client
    Online Status: self: true/4, peer: true/26
    Connections: total: 3742, ok: 3569, err: 173, active: 3, chans: 3, real1: 3, real2: 3
    Close Reasons: map[server\_close:2326 server\_close,both\_close:2326 connect\_timeout:151 both\_close:1 client\_close:1239]
    DataStream: req: 10192798, resp: 1863592824, r/s: 1/183, reqnet: 16906507, respnet: 2491620531, cost: 1.34
    goroutines: cur: 12, max: 88, min: 4
    memory: cur: 3400264, max: 6319608, min: 324144, total: 38373356736


### 服务端统计数据

    toxtun stats: server
    Online Status: self: true/5, peer: true/70
    Connections: total: 5950, ok: 5950, err: 0, active: 29, chans: 27, real1: 29, real2: 29
    DataStream: recv: 16694728, send: 2296697131, r/s: 1/138
    goroutines: cur: 38, max: 69, min: 8
    memory: cur: 65712744, max: 103190448, min: 318184, total: 47132792696
    
### 网络状态比较

##### 网1
lantern:

    21:10，[download]   5.4% of 285.44MiB at 40.66KiB/s ETA 01:53:17
    21:30, [download]   7.9% of 285.44MiB at 41.46KiB/s ETA 01:48:09
    23:20, [download]  53.8% of 285.44MiB at 67.47KiB/s ETA 33:22
    
toxtun:

    21:10，[download]  11.7% of 285.44MiB at 28.19KiB/s ETA 02:32:39
    21:30, [download]   2.5% of 259.04MiB at 58.25KiB/s ETA 01:13:59
    23:20, [download]  46.7% of 259.04MiB at 103.52KiB/s ETA 22:45

##### 网2

lantern:

    10:20, [download]   2.0% of 133.58MiB at 113.47KiB/s ETA 19:41
    10:50, [download]  86.1% of 259.04MiB at  1.36MiB/s ETA 00:26 (偏离值太大了)
    10:50, [download]   9.5% of 259.04MiB at 37.06KiB/s ETA 01:47:59
    11:50, [download]  40.7% of 259.04MiB at 522.40KiB/s ETA 05:01
    
toxtun:

    10:20, [download]  65.9% of 259.04MiB at 63.44KiB/s ETA 23:47
    10:50, [download]  98.9% of 133.58MiB at 108.42KiB/s ETA 00:13
    11:50, [download]   3.8% of 259.04MiB at 54.07KiB/s ETA 01:18:37


lantern偶尔使用时，看上去速度挺快，但如果持续使用，就不那么稳定了。

lantern在上网高峰网络状态最差的时候，表示的还比较稳定，但是也比最快的时候慢很多。

lantern的每一次连接，网络状况表现的都不太一样。

还有点差距。

