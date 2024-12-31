import asyncio
import websockets
import json
import random
from datetime import datetime, timezone


PORT = 8080

# Sends simulated portfolio data to connected clients
async def send_portfolio_data(websocket, path):
    if path != "/portfolio_stream":
        # Reject connections to invalid paths
        await websocket.close()
        print(f"Rejected connection with invalid path: {path}")
        return

    print(f"Client connected to {path}")

    try:
        while True:
            # Generate sample portfolio data
            portfolio_value = round(random.uniform(95000, 105000), 2)
            cash = portfolio_value if random.random() > 0.5 else round(random.uniform(50000, 95000), 2)
            position_size = 0 if cash == portfolio_value else random.randint(1, 10)
            position_price = 0.0 if position_size == 0 else round(random.uniform(1000, 2000), 2)

            # Generate current timestamp as a timezone-aware UTC datetime
            timestamp = datetime.now(timezone.utc).isoformat()

            # Create sample data
            data = {
                "portfolio_value": portfolio_value,
                "cash": cash,
                "position_size": position_size,
                "position_price": position_price,
                "datetime": timestamp
            }

            # Convert to JSON string
            message = json.dumps(data)

            # Send data to client
            await websocket.send(message)
            print(f"Sent: {message}")

            await asyncio.sleep(1)
            
    except websockets.ConnectionClosed:
        print("Client disconnected")

# Starts the WebSocket server
async def main():
    print(f"WebSocket server starting at ws://localhost:{PORT}")
    async with websockets.serve(send_portfolio_data, "localhost", PORT):
        await asyncio.Future()  # Run forever

if __name__ == "__main__":
    asyncio.run(main())
