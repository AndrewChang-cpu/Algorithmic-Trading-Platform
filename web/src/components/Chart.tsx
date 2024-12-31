import React from 'react';
import { Line } from 'react-chartjs-2';
import 'chart.js/auto';

interface ChartProps {
  labels: string[];
  data: number[];
}

const Chart: React.FC<ChartProps> = ({ labels, data }) => {
  const chartData = {
    labels,
    datasets: [
      {
        label: 'Portfolio Value',
        data,
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

  return <Line data={chartData} options={chartOptions} />;
};

export default Chart;
