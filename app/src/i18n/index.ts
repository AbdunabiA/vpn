import i18n from 'i18next';
import {initReactI18next} from 'react-i18next';
import {getLocales} from 'react-native-localize';

import en from './en.json';
import ru from './ru.json';

// Detect device language, default to Russian (primary audience)
const deviceLanguage = getLocales()[0]?.languageCode ?? 'ru';

i18n.use(initReactI18next).init({
  resources: {
    en: {translation: en},
    ru: {translation: ru},
  },
  lng: deviceLanguage === 'ru' ? 'ru' : 'en',
  fallbackLng: 'en',
  interpolation: {
    escapeValue: false,
  },
});

export default i18n;
