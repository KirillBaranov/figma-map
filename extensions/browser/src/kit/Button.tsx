import type { ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "ghost";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
};

export function Button({ variant = "primary", className, ...rest }: Props) {
  const variantClass =
    variant === "primary" ? "fm-button-primary" : variant === "secondary" ? "fm-button-secondary" : "fm-button-ghost";
  return <button className={`fm-button ${variantClass} ${className ?? ""}`.trim()} {...rest} />;
}
