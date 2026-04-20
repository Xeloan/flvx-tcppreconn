import * as React from "react";
import { createPortal } from "react-dom";
import { ChevronDownIcon } from "lucide-react";

import { FieldContainer, extractText, type FieldMetaProps } from "./shared";

import { Checkbox as BaseCheckbox } from "@/components/ui/checkbox";
import { cn } from "@/lib/utils";

type SelectionMode = "single" | "multiple";

type SelectionValue = Iterable<React.Key> | Set<React.Key> | Array<React.Key>;

interface OptionItem {
  disabled?: boolean;
  key: string;
  label: string;
}

interface ClassNameMap {
  base?: string;
  trigger?: string;
  [key: string]: string | undefined;
}

interface DropdownPosition {
  bottom?: number;
  left: number;
  maxHeight: number;
  top?: number;
  width: number;
}

export interface SelectProps<T = unknown> extends FieldMetaProps {
  children?: React.ReactNode | ((item: T) => React.ReactNode);
  className?: string;
  classNames?: ClassNameMap;
  disabledKeys?: SelectionValue;
  isDisabled?: boolean;
  items?: Iterable<T>;
  onChange?: (event: React.ChangeEvent<HTMLSelectElement>) => void;
  onClick?: (event: React.MouseEvent<HTMLSelectElement>) => void;
  onSelectionChange?: (keys: Set<React.Key>) => void;
  placeholder?: string;
  selectedKeys?: SelectionValue;
  selectionMode?: SelectionMode;
  size?: "sm" | "md" | "lg";
  variant?: string;
  dropdownPlacement?: "bottom" | "top";
}

export interface SelectItemProps {
  children?: React.ReactNode;
  description?: React.ReactNode;
  textValue?: string;
}

export function SelectItem(_props: SelectItemProps) {
  return null;
}

SelectItem.displayName = "HeroSelectItem";

function toSet(value?: SelectionValue) {
  if (!value) {
    return new Set<string>();
  }

  return new Set(Array.from(value).map((item) => String(item)));
}

function cleanReactKey(key: React.Key | null | undefined, fallback: string) {
  if (key == null) return fallback;
  const str = String(key);
  if (str.startsWith(".$")) {
    return str.substring(2);
  }
  if (str.startsWith(".")) {
    return str.substring(1);
  }
  return str;
}

function flattenOptionsFromNode(node: React.ReactNode, options: OptionItem[]) {
  React.Children.forEach(node, (child, index) => {
    if (child === null || child === undefined || typeof child === "boolean") {
      return;
    }
    if (Array.isArray(child)) {
      flattenOptionsFromNode(child, options);

      return;
    }
    if (React.isValidElement(child)) {
      if (child.type === React.Fragment) {
        flattenOptionsFromNode(child.props.children, options);

        return;
      }

      if (child.type === SelectItem) {
        const key = cleanReactKey(child.key, String(index));
        const props = child.props as SelectItemProps;

        options.push({
          key,
          label: props.textValue ?? extractText(props.children) ?? key,
        });

        return;
      }
    }
  });
}

function getOptions<T>(
  children: React.ReactNode | ((item: T) => React.ReactNode) | undefined,
  items: Iterable<T> | undefined,
) {
  const options: OptionItem[] = [];

  if (typeof children === "function" && items) {
    Array.from(items).forEach((item, index) => {
      const rendered = children(item);

      if (React.isValidElement(rendered) && rendered.type === SelectItem) {
        const key = cleanReactKey(rendered.key, String(index));
        const props = rendered.props as SelectItemProps;

        options.push({
          key,
          label: props.textValue ?? extractText(props.children) ?? key,
        });
      }
    });

    return options;
  }

  if (typeof children !== "function") {
    flattenOptionsFromNode(children, options);
  }

  return options;
}

function sizeClass(size: SelectProps["size"]) {
  if (size === "sm") {
    return "h-8 text-xs";
  }
  if (size === "lg") {
    return "h-10 text-base";
  }

  return "h-9 text-sm";
}

function textSizeClass(size: SelectProps["size"]) {
  if (size === "sm") {
    return "text-xs";
  }
  if (size === "lg") {
    return "text-base";
  }

  return "text-sm";
}

export function Select<T>({
  children,
  className,
  classNames,
  description,
  disabledKeys,
  errorMessage,
  isDisabled,
  isInvalid,
  isRequired,
  items,
  label,
  onChange,
  onClick,
  onSelectionChange,
  placeholder,
  selectedKeys,
  selectionMode = "single",
  size,
  dropdownPlacement = "bottom",
}: SelectProps<T>) {
  const generatedId = React.useId();
  const options = React.useMemo(
    () => getOptions(children, items),
    [children, items],
  );
  const containerRef = React.useRef<HTMLDivElement | null>(null);
  const listboxRef = React.useRef<HTMLDivElement | null>(null);
  const [isExpanded, setIsExpanded] = React.useState(false);
  const [dropdownPosition, setDropdownPosition] =
    React.useState<DropdownPosition | null>(null);
  const selected = React.useMemo(() => toSet(selectedKeys), [selectedKeys]);
  const disabled = React.useMemo(() => toSet(disabledKeys), [disabledKeys]);

  const updateDropdownPosition = React.useCallback(() => {
    if (selectionMode !== "multiple" || typeof window === "undefined") {
      return;
    }

    const container = containerRef.current;

    if (!container) {
      return;
    }

    const rect = container.getBoundingClientRect();
    const viewportHeight = window.innerHeight;
    const viewportWidth = window.innerWidth;
    const gap = 8;
    const inset = 8;
    const width = Math.min(rect.width, viewportWidth - inset * 2);
    const left = Math.min(
      Math.max(rect.left, inset),
      viewportWidth - width - inset,
    );

    if (dropdownPlacement === "top") {
      setDropdownPosition({
        bottom: Math.max(viewportHeight - rect.top + gap, inset),
        left,
        maxHeight: Math.max(rect.top - gap - inset, 120),
        width,
      });

      return;
    }

    setDropdownPosition({
      left,
      maxHeight: Math.max(viewportHeight - rect.bottom - gap - inset, 120),
      top: Math.min(rect.bottom + gap, viewportHeight - inset),
      width,
    });
  }, [dropdownPlacement, selectionMode]);

  React.useEffect(() => {
    if (!isExpanded) {
      return;
    }

    const handlePointerDown = (event: MouseEvent | TouchEvent) => {
      const container = containerRef.current;
      const listbox = listboxRef.current;

      if (!container) {
        return;
      }

      const target = event.target;

      if (!(target instanceof Node)) {
        return;
      }

      if (container.contains(target) || listbox?.contains(target)) {
        return;
      }

      setIsExpanded(false);
    };

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setIsExpanded(false);
      }
    };

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("touchstart", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);

    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("touchstart", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [isExpanded]);

  React.useEffect(() => {
    if (isDisabled) {
      setIsExpanded(false);
    }
  }, [isDisabled]);

  React.useLayoutEffect(() => {
    if (!isExpanded) {
      setDropdownPosition(null);

      return;
    }

    updateDropdownPosition();

    const handlePositionChange = () => {
      updateDropdownPosition();
    };

    window.addEventListener("resize", handlePositionChange);
    window.addEventListener("scroll", handlePositionChange, true);
    window.visualViewport?.addEventListener("resize", handlePositionChange);
    window.visualViewport?.addEventListener("scroll", handlePositionChange);

    return () => {
      window.removeEventListener("resize", handlePositionChange);
      window.removeEventListener("scroll", handlePositionChange, true);
      window.visualViewport?.removeEventListener(
        "resize",
        handlePositionChange,
      );
      window.visualViewport?.removeEventListener(
        "scroll",
        handlePositionChange,
      );
    };
  }, [isExpanded, updateDropdownPosition]);

  const selectedArray = Array.from(selected);
  const singleValue = selectedArray[0] ?? "";
  const optionLabelMap = React.useMemo(() => {
    return new Map(options.map((option) => [option.key, option.label]));
  }, [options]);
  const resolvedSelectedValues = selectedArray.map((key) => {
    const keyText = String(key);

    return optionLabelMap.get(keyText) ?? keyText;
  });
  const selectedFullText = resolvedSelectedValues.join("、");
  const selectedText =
    selectedArray.length > 0 ? selectedFullText : (placeholder ?? "请选择");

  const updateMultipleSelection = (key: string, checked?: boolean) => {
    if (isDisabled || disabled.has(key)) {
      return;
    }

    const next = new Set(selected);
    const shouldSelect =
      typeof checked === "boolean" ? checked : !next.has(key);

    if (shouldSelect) {
      next.add(key);
    } else {
      next.delete(key);
    }

    onSelectionChange?.(next);
  };

  const handleChange = (event: React.ChangeEvent<HTMLSelectElement>) => {
    onChange?.(event);

    if (!onSelectionChange) {
      return;
    }

    if (selectionMode === "multiple") {
      const values = Array.from(event.target.selectedOptions).map(
        (option) => option.value,
      );

      onSelectionChange(new Set(values));

      return;
    }

    if (!event.target.value) {
      onSelectionChange(new Set());

      return;
    }

    onSelectionChange(new Set([event.target.value]));
  };

  const renderMultipleListbox = () => {
    if (!isExpanded || !dropdownPosition || typeof document === "undefined") {
      return null;
    }

    return createPortal(
      <div
        ref={listboxRef}
        className={cn(
          "fixed z-[60] space-y-1 overflow-y-auto rounded-md border border-divider bg-background p-2 shadow-md pointer-events-auto",
        )}
        id={`${generatedId}-listbox`}
        role="listbox"
        style={dropdownPosition}
        onClick={(e) => {
          // Stop react click bubbling up to the form
          e.stopPropagation();
        }}
        onPointerDown={(e) => {
          e.stopPropagation();
        }}
      >
        {options.length === 0 ? (
          <div
            className={cn("px-2 py-1 text-default-500", textSizeClass(size))}
          >
            暂无可选项
          </div>
        ) : (
          options.map((option) => {
            const optionDisabled = isDisabled || disabled.has(option.key);

            return (
              <div
                key={option.key}
                className={cn(
                  "flex items-center gap-2 rounded-md px-2 py-1.5",
                  optionDisabled
                    ? "cursor-not-allowed opacity-60"
                    : "hover:bg-default-100",
                )}
              >
                <BaseCheckbox
                  checked={selected.has(option.key)}
                  disabled={optionDisabled}
                  onCheckedChange={(value) =>
                    updateMultipleSelection(option.key, value === true)
                  }
                />
                <button
                  className={cn(
                    "min-w-0 flex-1 truncate text-left text-foreground",
                    textSizeClass(size),
                    optionDisabled ? "cursor-not-allowed" : "cursor-pointer",
                  )}
                  disabled={optionDisabled}
                  type="button"
                  onClick={() => updateMultipleSelection(option.key)}
                >
                  {option.label}
                </button>
              </div>
            );
          })
        )}
      </div>,
      document.body,
    );
  };

  return (
    <FieldContainer
      className={classNames?.base}
      description={description}
      errorMessage={errorMessage}
      id={generatedId}
      isInvalid={isInvalid}
      isRequired={isRequired}
      label={label}
    >
      {selectionMode === "multiple" ? (
        <div ref={containerRef} className={cn("relative w-full", className)}>
          <button
            aria-controls={`${generatedId}-listbox`}
            aria-expanded={isExpanded}
            aria-haspopup="listbox"
            className={cn(
              "flex w-full min-w-0 items-center gap-2 overflow-hidden rounded-md border border-input bg-background px-3 py-2 text-left shadow-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              isDisabled ? "cursor-not-allowed opacity-60" : "",
              classNames?.trigger,
            )}
            disabled={isDisabled}
            id={generatedId}
            type="button"
            onClick={() => setIsExpanded((prev) => !prev)}
          >
            <span
              className={cn(
                "block min-w-0 flex-1 truncate",
                textSizeClass(size),
                selectedArray.length > 0
                  ? "text-foreground"
                  : "text-default-500",
              )}
              title={selectedArray.length > 0 ? selectedFullText : undefined}
            >
              {selectedText}
            </span>
            <ChevronDownIcon
              className={cn(
                "h-4 w-4 flex-shrink-0 text-default-500 transition-transform",
                isExpanded ? "rotate-180" : "rotate-0",
              )}
            />
          </button>
          {renderMultipleListbox()}
        </div>
      ) : (
        <select
          className={cn(
            "w-full rounded-md border border-input bg-background px-3 py-2 text-foreground shadow-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:[color-scheme:dark]",
            sizeClass(size),
            classNames?.trigger,
            className,
          )}
          disabled={isDisabled}
          id={generatedId}
          required={isRequired}
          value={singleValue}
          onChange={handleChange}
          onClick={onClick}
        >
          <option value="">{placeholder ?? "请选择"}</option>
          {options.map((option) => (
            <option
              key={option.key}
              className="bg-background text-foreground"
              disabled={disabled.has(option.key)}
              value={option.key}
            >
              {option.label}
            </option>
          ))}
        </select>
      )}
    </FieldContainer>
  );
}
