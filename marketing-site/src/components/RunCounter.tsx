"use client";

import { useEffect, useState } from "react";

const MAX_COUNT = 9999;
const TICK_MS = 250;

export function RunCounter() {
  const [count, setCount] = useState(0);
  const [reduceMotion, setReduceMotion] = useState(false);

  useEffect(() => {
    const media = window.matchMedia("(prefers-reduced-motion: reduce)");
    const updateReduceMotion = () => setReduceMotion(media.matches);

    updateReduceMotion();
    if (typeof media.addEventListener === "function") {
      media.addEventListener("change", updateReduceMotion);
      return () => media.removeEventListener("change", updateReduceMotion);
    }

    media.addListener(updateReduceMotion);
    return () => media.removeListener(updateReduceMotion);
  }, []);

  useEffect(() => {
    if (reduceMotion) {
      return;
    }

    const timer = window.setInterval(() => {
      setCount((current) => (current >= MAX_COUNT ? 0 : current + 1));
    }, TICK_MS);

    return () => window.clearInterval(timer);
  }, [reduceMotion]);

  return <strong>{count.toString().padStart(4, "0")}</strong>;
}
