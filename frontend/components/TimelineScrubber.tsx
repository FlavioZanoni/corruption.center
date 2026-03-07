"use client";

import { useCallback, useRef, useState, useEffect } from "react";
import { useAppStore } from "@/lib/store";

const MIN_YEAR = 2000;
const MAX_YEAR = 2025;
const YEAR_MARKERS = [2000, 2005, 2010, 2015, 2020, 2025];

export function TimelineScrubber() {
  const { timelineRange, setTimelineRange } = useAppStore();

  const trackRef = useRef<HTMLDivElement>(null);
  const [dragging, setDragging] = useState<"from" | "to" | null>(null);

  const yearToPercent = (year: number) =>
    ((year - MIN_YEAR) / (MAX_YEAR - MIN_YEAR)) * 100;

  const percentToYear = (pct: number): number => {
    const raw = MIN_YEAR + (pct / 100) * (MAX_YEAR - MIN_YEAR);
    return Math.round(Math.max(MIN_YEAR, Math.min(MAX_YEAR, raw)));
  };

  const getYearFromEvent = useCallback((clientX: number): number => {
    if (!trackRef.current) return MIN_YEAR;
    const rect = trackRef.current.getBoundingClientRect();
    const pct = ((clientX - rect.left) / rect.width) * 100;
    return percentToYear(pct);
  }, []);

  const handleMouseMove = useCallback(
    (e: MouseEvent) => {
      if (!dragging) return;
      const year = getYearFromEvent(e.clientX);
      if (dragging === "from") {
        setTimelineRange({
          from: Math.min(year, timelineRange.to - 1),
          to: timelineRange.to,
        });
      } else {
        setTimelineRange({
          from: timelineRange.from,
          to: Math.max(year, timelineRange.from + 1),
        });
      }
    },
    [dragging, getYearFromEvent, timelineRange, setTimelineRange],
  );

  const handleMouseUp = useCallback(() => setDragging(null), []);

  useEffect(() => {
    if (dragging) {
      document.addEventListener("mousemove", handleMouseMove);
      document.addEventListener("mouseup", handleMouseUp);
    }
    return () => {
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
    };
  }, [dragging, handleMouseMove, handleMouseUp]);

  const handleTouchMove = useCallback(
    (e: React.TouchEvent) => {
      if (!dragging) return;
      const year = getYearFromEvent(e.touches[0].clientX);
      if (dragging === "from") {
        setTimelineRange({
          from: Math.min(year, timelineRange.to - 1),
          to: timelineRange.to,
        });
      } else {
        setTimelineRange({
          from: timelineRange.from,
          to: Math.max(year, timelineRange.from + 1),
        });
      }
    },
    [dragging, getYearFromEvent, timelineRange, setTimelineRange],
  );

  const fromPct = yearToPercent(timelineRange.from);
  const toPct = yearToPercent(timelineRange.to);

  return (
    <div className="absolute bottom-0 left-0 right-0 z-20 flex justify-center pb-4 pt-3 bg-linear-to-t from-bg to-transparent">
      <div className="w-full max-w-2xl px-8">
      <div className="relative h-4 mb-1 select-none">
        {YEAR_MARKERS.map((year) => (
          <span
            key={year}
            className="absolute text-[9px] font-mono text-[#444444] -translate-x-1/2"
            style={{ left: `${yearToPercent(year)}%` }}
          >
            {year}
          </span>
        ))}
      </div>

      <div
        ref={trackRef}
        className="relative h-0.75 bg-border rounded-full cursor-pointer"
        onClick={(e) => {
          const year = getYearFromEvent(e.clientX);
          const distFrom = Math.abs(year - timelineRange.from);
          const distTo = Math.abs(year - timelineRange.to);
          if (distFrom < distTo) {
            setTimelineRange({
              from: Math.min(year, timelineRange.to - 1),
              to: timelineRange.to,
            });
          } else {
            setTimelineRange({
              from: timelineRange.from,
              to: Math.max(year, timelineRange.from + 1),
            });
          }
        }}
      >
        <div
          className="absolute top-0 bottom-0 bg-[#cc2222] opacity-60 rounded-full"
          style={{ left: `${fromPct}%`, right: `${100 - toPct}%` }}
        />

        {YEAR_MARKERS.map((year) => (
          <div
            key={year}
            className="absolute top-1/2 -translate-y-1/2 w-px h-2 bg-[#333333] -translate-x-1/2"
            style={{ left: `${yearToPercent(year)}%` }}
          />
        ))}

        {/* from handle */}
        <div
          className="absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-3.5 h-3.5 rounded-full bg-text border-2 border-bg cursor-grab active:cursor-grabbing z-10 hover:bg-white transition-colors"
          style={{ left: `${fromPct}%` }}
          onMouseDown={(e) => {
            e.stopPropagation();
            setDragging("from");
          }}
          onTouchStart={(e) => {
            e.stopPropagation();
            setDragging("from");
          }}
          onTouchMove={handleTouchMove}
          onTouchEnd={() => setDragging(null)}
        />

        {/* to handle */}
        <div
          className="absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-3.5 h-3.5 rounded-full bg-text border-2 border-bg cursor-grab active:cursor-grabbing z-10 hover:bg-white transition-colors"
          style={{ left: `${toPct}%` }}
          onMouseDown={(e) => {
            e.stopPropagation();
            setDragging("to");
          }}
          onTouchStart={(e) => {
            e.stopPropagation();
            setDragging("to");
          }}
          onTouchMove={handleTouchMove}
          onTouchEnd={() => setDragging(null)}
        />
      </div>

      <div className="mt-2 flex justify-center">
        <span className="text-[10px] font-mono text-[#888888]">
          {timelineRange.from} — {timelineRange.to}
        </span>
      </div>
      </div>
    </div>
  );
}
