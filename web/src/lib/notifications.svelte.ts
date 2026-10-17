export type NotificationType = "success" | "error" | "warning" | "info";

export type Notification = {
  id: string;
  message: string;
  type: NotificationType;
  title: string;
  dismissible: boolean;
};

type NotifyOptions = { title?: string; dismissible?: boolean; timeoutMs?: number };

// Toast list rendered by the shell layout, mirroring upstream's notification store shape
// (type success | error | warning | info, optional title, dismissible).
// ponytail: auto-dismiss defaults to 5s here; align with upstream's notificationStore when it is
// compared against the reference checkout.
let seq = 0;

export const notifications = $state<{ items: Notification[] }>({ items: [] });

export function notify(message: string, type: NotificationType = "info", options: NotifyOptions = {}): string {
  const { title = "", dismissible = true, timeoutMs = 5000 } = options;
  const id = `n${++seq}`;
  notifications.items = [...notifications.items, { id, message, type, title, dismissible }];
  if (timeoutMs > 0) setTimeout(() => removeNotification(id), timeoutMs);
  return id;
}

export function removeNotification(id: string): void {
  notifications.items = notifications.items.filter((item) => item.id !== id);
}

export const toast = {
  success: (message: string, options?: NotifyOptions) => notify(message, "success", options),
  error: (message: string, options?: NotifyOptions) => notify(message, "error", options),
  warning: (message: string, options?: NotifyOptions) => notify(message, "warning", options),
  info: (message: string, options?: NotifyOptions) => notify(message, "info", options),
};
