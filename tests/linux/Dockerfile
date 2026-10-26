FROM debian:bookworm-slim
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends libwebkit2gtk-4.1-0 xvfb openbox x11vnc novnc websockify dbus-x11 ca-certificates fonts-noto-cjk gstreamer1.0-libav pulseaudio && rm -rf /var/lib/apt/lists/*
RUN useradd -m -u 1000 tester
COPY start-linux.sh /start-linux.sh
USER tester
ENV DISPLAY=:99 XDG_RUNTIME_DIR=/tmp/runtime-tester
CMD ["dbus-run-session", "--", "sh", "/start-linux.sh"]
