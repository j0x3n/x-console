export const MIN_PASSWORD = 10;

/** 修改密码表单的错误提示。没问题时返回空字符串。 */
export function passwordFormError(form: {
  oldPassword: string;
  newPassword: string;
  confirm: string;
}): string {
  if (form.newPassword.length < MIN_PASSWORD)
    return `新密码至少 ${MIN_PASSWORD} 位`;
  if (form.newPassword !== form.confirm) return "两次输入的新密码不一致";
  if (form.newPassword === form.oldPassword) return "新密码和旧密码一样";
  return "";
}
