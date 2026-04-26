import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import type { ReactElement, ReactNode } from "react";

type TooltipProps = {
  content?: ReactNode;
  children: ReactElement;
  side?: "top" | "right" | "bottom" | "left";
  align?: "start" | "center" | "end";
};

export function PindocTooltipProvider({ children }: { children: ReactNode }) {
  return (
    <TooltipPrimitive.Provider delayDuration={250} skipDelayDuration={100}>
      {children}
    </TooltipPrimitive.Provider>
  );
}

export function Tooltip({ content, children, side = "top", align = "center" }: TooltipProps) {
  if (!content) return children;
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          className="pindoc-tooltip"
          side={side}
          align={align}
          sideOffset={6}
          collisionPadding={10}
        >
          {content}
          <TooltipPrimitive.Arrow className="pindoc-tooltip__arrow" width={9} height={5} />
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
