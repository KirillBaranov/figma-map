import type { InputHTMLAttributes, TextareaHTMLAttributes, ReactNode } from "react";

type LabelProps = {
  label: string;
  hint?: string;
};

function FieldShell({ label, hint, children }: LabelProps & { children: ReactNode }) {
  return (
    <div className="fm-field">
      <label>{label}</label>
      {children}
      {hint && <p>{hint}</p>}
    </div>
  );
}

export function TextField({
  label,
  hint,
  ...rest
}: LabelProps & InputHTMLAttributes<HTMLInputElement>) {
  return (
    <FieldShell label={label} hint={hint}>
      <input className="fm-input" {...rest} />
    </FieldShell>
  );
}

export function TextAreaField({
  label,
  hint,
  ...rest
}: LabelProps & TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <FieldShell label={label} hint={hint}>
      <textarea className="fm-textarea" {...rest} />
    </FieldShell>
  );
}
