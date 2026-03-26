"use client";

import { useEffect, useState } from "react";

const MAX_COUNT = 9999;
const TICK_MS = 250;

export function RunCounter() {
  const [count, setCount] = useState(0);

  useEffect(() => {
    const timer = window.setInterval(() => {
      setCount((current) => (current >= MAX_COUNT ? 0 : current + 1));
    }, TICK_MS);

    return () => window.clearInterval(timer);
  }, []);

  return <strong>{count.toString().padStart(4, "0")}</strong>;
}
