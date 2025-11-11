import ws from 'k6/ws';
import { Counter, Trend } from 'k6/metrics';

// --- Custom metrics for resume-worthy stats ---
export let wsConnects = new Counter('ws_connects');
export let wsConnectErrors = new Counter('ws_connect_errors');
export let wsMsgsSent = new Counter('ws_msgs_sent');
export let wsMsgsReceived = new Counter('ws_msgs_received');
export let wsConnectTime = new Trend('ws_connect_time_ms');

// --- Configuration ---
const wsUrl = __ENV.WS_URL;
if (!wsUrl) throw new Error('Please specify WS_URL via -e WS_URL=...');

export const options = {
  stages: [
    { duration: '1m', target: 50 },
    { duration: '2m', target: 200 },
    { duration: '2m', target: 500 },
    { duration: '4m', target: 1000 },
    { duration: '1m', target: 0 },
  ],
  thresholds: {
    ws_connects: ['rate>0.99'],       // 99% successful connections
    ws_connect_errors: ['count<10'],  // at most 10 connection errors
    ws_msgs_sent: ['count>1000'],       // messages sent during test
    ws_msgs_received: ['count>1000'],   // messages received during test
  },
};

// --- Default function run by each VU ---
export default function () {
  const start = Date.now();
  const res = ws.connect(wsUrl, { timeout: '10s' }, function (socket) {
    socket.on('open', () => {
      wsConnects.add(1);
      const duration = Date.now() - start;
      wsConnectTime.add(duration);
      socket.send(`User_${__VU} joined`);
      wsMsgsSent.add(1);

      // Send a message every 2-4 seconds
      socket.setInterval(() => {
        socket.send(`User_${__VU}: message`);
        wsMsgsSent.add(1);
      }, Math.random() * 2000 + 2000);
    });

    socket.on('message', (msg) => {
      wsMsgsReceived.add(1); 
    });


    socket.on('error', (e) => {
      wsConnectErrors.add(1);
      console.error(`VU ${__VU}: WebSocket error`, e.error());
    });

    socket.on('close', () => {
      console.log(`VU ${__VU}: disconnected`);
    });
  });

  // Optional check for first connection attempt
  if (!res || res.status !== 101) wsConnectErrors.add(1);
}