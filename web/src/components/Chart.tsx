import React, { useEffect, useRef, useState } from 'react';
import { Line } from 'react-chartjs-2';
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
} from 'chart.js';

// Register the components and scales
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend
);

interface ChartProps {
  userId: string;
}

const Chart: React.FC<ChartProps> = ({ userId }) => {
  const [chartLabels, setLabels] = useState<string[]>([]);
  const [chartData, setValues] = useState<number[]>([]);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const wsUrl = `ws://localhost:8080/portfolio_stream`;
    wsRef.current = new WebSocket(wsUrl);

    wsRef.current.onmessage = (event) => {
      const data = JSON.parse(event.data);
      const time = new Date(data.datetime).toLocaleTimeString();
      setLabels((prev) => [...prev, time].slice(-20));
      setValues((prev) => [...prev, data.portfolio_value].slice(-20));
    };

    return () => wsRef.current?.close();
  }, [userId]);

  const chartDataConfig = {
    labels: chartLabels,
    datasets: [
      {
        label: 'Portfolio Value',
        data: chartData,
        borderColor: 'rgba(75, 192, 192, 1)',
        backgroundColor: 'rgba(75, 192, 192, 0.2)',
        borderWidth: 2,
        tension: 0.3,
      },
    ],
  };

  const chartOptions = {
    scales: {
      x: { title: { display: true, text: 'Time' } },
      y: { title: { display: true, text: 'Portfolio Value' } },
    },
  };

  return <Line data={chartDataConfig} options={chartOptions} />;
};

export default Chart;