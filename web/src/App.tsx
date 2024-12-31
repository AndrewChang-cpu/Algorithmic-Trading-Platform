import React, { useEffect, useRef, useState } from 'react';
import Chart from './components/Chart';

const App: React.FC = () => {
  const [labels, setLabels] = useState<string[]>([]);
  const [values, setValues] = useState<number[]>([]);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const wsUrl = 'ws://localhost:8080/portfolio_stream';
    wsRef.current = new WebSocket(wsUrl);

    wsRef.current.onmessage = (event) => {
      const data = JSON.parse(event.data);
      const time = new Date(data.datetime).toLocaleTimeString();
      setLabels((prev) => [...prev, time].slice(-20));
      setValues((prev) => [...prev, data.portfolio_value].slice(-20));
    };

    return () => wsRef.current?.close();
  }, []);

  return (
    <div>
      <h1>Portfolio Value Chart</h1>
      <Chart labels={labels} data={values} />
    </div>
  );
};

export default App;
