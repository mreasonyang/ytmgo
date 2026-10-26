#!/bin/sh
set -eu
mkdir -p "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"
# This display belongs only to this disposable test container. A stopped
# container can leave the X lock/socket behind, so clear them before relaunch.
rm -f /tmp/.X99-lock /tmp/.X11-unix/X99
Xvfb :99 -screen 0 1440x1000x24 > /tmp/xvfb.log 2>&1 &
display_pid=$!
attempt=0
while [ ! -S /tmp/.X11-unix/X99 ]; do
  kill -0 "$display_pid"
  attempt=$((attempt + 1))
  [ "$attempt" -lt 100 ] || exit 1
  sleep 0.1
done
openbox > /tmp/openbox.log 2>&1 &
x11vnc -display :99 -localhost -nopw -forever -shared > /tmp/vnc.log 2>&1 &
websockify --web=/usr/share/novnc/ 6080 localhost:5900 > /tmp/websockify.log 2>&1 &
pulseaudio --start --exit-idle-time=-1
pactl load-module module-null-sink sink_name=e2e > /tmp/pulse-sink.log
/target/ytmgo > /tmp/ytmgo-runtime.log 2>&1 &
echo $! > /tmp/ytmgo.pid
wait
