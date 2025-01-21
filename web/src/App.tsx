import React from 'react';
import Chart from './components/Chart';
import './App.css';

const App: React.FC = () => {
  return (
    <div>
      <h1>Portfolio Value Chart</h1>
      <div className="chart-grid">
        {Array.from({ length: 8 }).map((_, index) => (
          <div key={index} className="chart-container">
            <Chart userId="TODO" />
          </div>
        ))}
      </div>
    </div>
  );
};

export default App;