import React, { useEffect, useRef, useState } from 'react';
import Chart from './components/Chart';

const App: React.FC = () => {
  return (
    <div>
      <h1>Portfolio Value Chart</h1>
      <Chart userId='TODO' />
    </div>
  );
};

export default App;
