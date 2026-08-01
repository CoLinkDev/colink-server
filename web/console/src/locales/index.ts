import { createI18n } from 'vue-i18n'
import en from './en'
import zhCN from './zh-CN'
import zhTW from './zh-TW'
import ja from './ja'
import ko from './ko'
import es from './es'
import de from './de'
import ru from './ru'

const getBrowserLocale = () => {
  const lang = navigator.language
  if (lang.startsWith('zh-CN') || lang === 'zh') return 'zh-CN'
  if (lang.startsWith('zh')) return 'zh-TW'
  if (lang.startsWith('ja')) return 'ja'
  if (lang.startsWith('ko')) return 'ko'
  if (lang.startsWith('es')) return 'es'
  if (lang.startsWith('de')) return 'de'
  if (lang.startsWith('ru')) return 'ru'
  return 'en'
}

const i18n = createI18n({
  legacy: false,
  locale: localStorage.getItem('locale') || getBrowserLocale(),
  fallbackLocale: 'en',
  messages: {
    en,
    'zh-CN': zhCN,
    'zh-TW': zhTW,
    ja,
    ko,
    es,
    de,
    ru
  }
})

export default i18n
