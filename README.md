## XIVI

Description here...

### ToDo

- [ ] Fix all the things

### INSTALL & RUN

1. Install libvips (https://github.com/libvips/libvips)
2. Install gcc and pkg-config
3. Install GStreamer development files (https://github.com/go-gst/go-gst)
4. Setup: npm run setup-all
5. Rename `.env.example` to `.env` and fill it with your environment values.
6. Build it: npm run build-all
7. Run it: npm run serve-win
8. Go to your API Docs page: [127.0.0.1:8080/swagger/index.html](http://127.0.0.1:8080/swagger/index.html)

### WINDOWS DEV

### INSTALL & RUN

1. Install libvips (https://github.com/libvips/libvips)
2. Install mingw (https://community.chocolatey.org/packages/mingw)
3. Install pkgconfig (choco install pkgconfiglite)
4. Install the latest "development installer" for your MinGW architecture (https://gstreamer.freedesktop.org/download)
5. Setup: npm run setup-all
6. Rename `.env.example` to `.env` and fill it with your environment values.
7. Build it: npm run build-all
8. Run it: npm run serve-win
9. Go to your API Docs page: [127.0.0.1:8080/swagger/index.html](http://127.0.0.1:8080/swagger/index.html)

Finally, to compile the application you'll have to manually set your `PKG_CONFIG_PATH` to where you installed the GStreamer development files.
For example, if you installed GStreamer to `C:\gstreamer`:

```ps
PS> $env:PKG_CONFIG_PATH='C:\gstreamer\1.0\mingw_x86_64\lib\pkgconfig'
PS> go build .
```
