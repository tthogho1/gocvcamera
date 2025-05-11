import WebSocket, { WebSocketServer } from 'ws';

const wss = new WebSocketServer({ port: 8080 });

console.log('WebSocket server started on port 8080');

interface Client {
  ws: WebSocket;
  id: string;
}

const clients: Client[] = [];

wss.on('connection', ws => {
  const clientId = generateId();
  const client: Client = { ws, id: clientId };
  clients.push(client);
  console.log(`Client connected: ${clientId}, Total clients: ${clients.length}`);

  ws.on('message', message => {
    console.log(`Received message from ${clientId}: ${message}`);
    // メッセージを送信元以外のすべてのクライアントに中継
    clients.forEach(c => {
      if (c.id !== clientId && c.ws.readyState === WebSocket.OPEN) {
        c.ws.send(`[${clientId}]: ${message}`);
      }
    });
  });

  ws.on('close', () => {
    clients.splice(clients.indexOf(client), 1);
    console.log(`Client disconnected: ${clientId}, Remaining clients: ${clients.length}`);
  });

  ws.on('error', error => {
    console.error(`WebSocket error for client ${clientId}: ${error}`);
    clients.splice(clients.indexOf(client), 1);
  });

  // 接続時にクライアントIDを送信
  ws.send(JSON.stringify({ type: 'connected', id: clientId }));
});

function generateId(): string {
  return Math.random().toString(36).substring(2, 15);
}
