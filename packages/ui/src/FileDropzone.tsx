"use client";

import { useRef, useState, type DragEvent, type ReactNode } from "react";
import clsx from "clsx";

export interface FileDropzoneProps {
  /** Called with the files the user dropped or picked. */
  onFiles: (files: File[]) => void;
  /** Passed to the file input, e.g. ".pdf,.png,.jpg,.docx". Advisory only — the server checks content. */
  accept?: string;
  multiple?: boolean;
  disabled?: boolean;
  /** Text inside the drop area (localized by the caller). */
  prompt: ReactNode;
  /** Accessible name of the hidden file input. */
  inputLabel: string;
}

/** Drag-and-drop area that also opens the file picker on click or Enter/Space (PLT-09). */
export function FileDropzone({ onFiles, accept, multiple = false, disabled, prompt, inputLabel }: FileDropzoneProps) {
  const input = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);

  const take = (list: FileList | null) => {
    if (!list || disabled) return;
    const files = Array.from(list);
    if (files.length) onFiles(multiple ? files : files.slice(0, 1));
  };
  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setOver(false);
    take(e.dataTransfer.files);
  };

  return (
    <div
      role="button"
      tabIndex={disabled ? -1 : 0}
      aria-disabled={disabled}
      onClick={() => input.current?.click()}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          input.current?.click();
        }
      }}
      onDragOver={(e) => {
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={onDrop}
      className={clsx(
        "flex min-h-28 cursor-pointer items-center justify-center rounded-md border-2 border-dashed p-6 text-center text-sm transition-colors",
        over ? "border-slate-900 bg-slate-100" : "border-slate-300 bg-white hover:bg-slate-50",
        disabled && "pointer-events-none opacity-50",
      )}
    >
      {prompt}
      <input
        ref={input}
        type="file"
        className="sr-only"
        aria-label={inputLabel}
        accept={accept}
        multiple={multiple}
        onChange={(e) => {
          take(e.target.files);
          e.target.value = "";
        }}
      />
    </div>
  );
}

export interface ProgressBarProps {
  /** 0..1, or undefined for an indeterminate bar (e.g. while the virus scan runs). */
  value?: number;
  label: string;
}

export function ProgressBar({ value, label }: ProgressBarProps) {
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={value === undefined ? undefined : Math.round(value * 100)}
      className="h-1.5 w-full overflow-hidden rounded bg-slate-200"
    >
      <div
        className={clsx("h-full bg-slate-900 transition-all", value === undefined && "w-1/3 animate-pulse")}
        style={value === undefined ? undefined : { width: `${Math.round(value * 100)}%` }}
      />
    </div>
  );
}
