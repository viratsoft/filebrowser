type IToastSuccess = (message: string) => void;
type IToastError = (
  error: Error | string,
  displayReport?: boolean,
  toastClassName?: string
) => void;
