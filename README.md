## XIVI
Description here... 
### ToDo
- [ ] Fix all the things

### INSTALL & RUN
1. Install libvips (https://github.com/libvips/libvips)
2. Install gcc and pkg-config
3. Install GStreamer development files (https://github.com/go-gst/go-gst)
2. Setup: npm run setup-all
3. Rename `.env.example` to `.env` and fill it with your environment values.
4. Build it: npm run build-all
5. Run it: npm run serve-win
6. Go to your API Docs page: [127.0.0.1:8080/swagger/index.html](http://127.0.0.1:8080/swagger/index.html)

### WINDOWS DEV
### INSTALL & RUN
1. Install libvips (https://github.com/libvips/libvips)
2. Install mingw (https://community.chocolatey.org/packages/mingw)
3. Install pkgconfig (choco install pkgconfiglite)
4. Install the latest "development installer" for your MinGW architecture (https://gstreamer.freedesktop.org/download)
2. Setup: npm run setup-all
3. Rename `.env.example` to `.env` and fill it with your environment values.
4. Build it: npm run build-all
5. Run it: npm run serve-win
6. Go to your API Docs page: [127.0.0.1:8080/swagger/index.html](http://127.0.0.1:8080/swagger/index.html)

Finally, to compile the application you'll have to manually set your `PKG_CONFIG_PATH` to where you installed the GStreamer development files.
For example, if you installed GStreamer to `C:\gstreamer`:

```ps
PS> $env:PKG_CONFIG_PATH='C:\gstreamer\1.0\mingw_x86_64\lib\pkgconfig'
PS> go build .
```