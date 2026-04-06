import type { ToastType } from './ToastType';

export interface Toast {
    id: number;
    type: ToastType;
    message: string;
    duration: number | null;
}
