Synthetic H.264/AAC test pattern generated locally with FFmpeg; no third-party media.
The test serves the playlist at `/stream/hls/fixture` without an extension, matching Xivi.

Regenerate from this directory:

```sh
ffmpeg -f lavfi -i testsrc2=size=320x180:rate=15 -f lavfi -i sine=frequency=440:sample_rate=48000 -t 8 -c:v libx264 -preset ultrafast -crf 34 -pix_fmt yuv420p -g 120 -sc_threshold 0 -c:a aac -b:a 48k -hls_time 8 -hls_list_size 0 -hls_segment_filename segment%03d.ts playlist.m3u8
```
