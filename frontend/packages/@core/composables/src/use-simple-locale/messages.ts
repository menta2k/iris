export type Locale = 'bg-BG' | 'en-US';

export const messages: Record<Locale, Record<string, string>> = {
  'en-US': {
    cancel: 'Cancel',
    collapse: 'Collapse',
    confirm: 'Confirm',
    expand: 'Expand',
    reset: 'Reset',
    submit: 'Submit',
  },
  'bg-BG': {
    cancel: 'Отказ',
    collapse: 'Сгъни',
    confirm: 'Потвърди',
    expand: 'Разгъни',
    reset: 'Нулирай',
    submit: 'Изпрати',
  },
};

export const getMessages = (locale: Locale) => {
  // Fall back to en-US for any locale we haven't translated yet.
  return messages[locale] ?? messages['en-US'];
};
