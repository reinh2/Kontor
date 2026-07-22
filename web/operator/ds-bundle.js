/* @ds-bundle: {"format":4,"namespace":"KontorKanonDesignSystem_452420","components":[{"name":"Sparkline","sourcePath":"components/charts/Chart.jsx"},{"name":"BarChart","sourcePath":"components/charts/Chart.jsx"},{"name":"DonutChart","sourcePath":"components/charts/Chart.jsx"},{"name":"Chart","sourcePath":"components/charts/Chart.jsx"},{"name":"Badge","sourcePath":"components/core/Badge.jsx"},{"name":"Button","sourcePath":"components/core/Button.jsx"},{"name":"Card","sourcePath":"components/core/Card.jsx"},{"name":"CardHeader","sourcePath":"components/core/Card.jsx"},{"name":"IconButton","sourcePath":"components/core/IconButton.jsx"},{"name":"CodeBlock","sourcePath":"components/data/CodeBlock.jsx"},{"name":"DataTable","sourcePath":"components/data/DataTable.jsx"},{"name":"KeyValue","sourcePath":"components/data/KeyValue.jsx"},{"name":"KeyValueList","sourcePath":"components/data/KeyValue.jsx"},{"name":"EmptyState","sourcePath":"components/feedback/EmptyState.jsx"},{"name":"ErrorState","sourcePath":"components/feedback/ErrorState.jsx"},{"name":"Skeleton","sourcePath":"components/feedback/Skeleton.jsx"},{"name":"SkeletonText","sourcePath":"components/feedback/Skeleton.jsx"},{"name":"SkeletonRow","sourcePath":"components/feedback/Skeleton.jsx"},{"name":"Checkbox","sourcePath":"components/forms/Checkbox.jsx"},{"name":"Input","sourcePath":"components/forms/Input.jsx"},{"name":"Select","sourcePath":"components/forms/Select.jsx"},{"name":"Switch","sourcePath":"components/forms/Switch.jsx"},{"name":"Avatar","sourcePath":"components/identity/Avatar.jsx"},{"name":"AvatarGroup","sourcePath":"components/identity/Avatar.jsx"},{"name":"Sidebar","sourcePath":"components/navigation/Sidebar.jsx"},{"name":"SidebarSection","sourcePath":"components/navigation/Sidebar.jsx"},{"name":"SidebarItem","sourcePath":"components/navigation/Sidebar.jsx"},{"name":"Tabs","sourcePath":"components/navigation/Tabs.jsx"},{"name":"Drawer","sourcePath":"components/overlays/Drawer.jsx"},{"name":"Modal","sourcePath":"components/overlays/Modal.jsx"},{"name":"Toast","sourcePath":"components/overlays/Toast.jsx"},{"name":"ToastStack","sourcePath":"components/overlays/Toast.jsx"},{"name":"Tooltip","sourcePath":"components/overlays/Tooltip.jsx"},{"name":"WeekCalendar","sourcePath":"components/structure/Calendar.jsx"},{"name":"DayCalendar","sourcePath":"components/structure/Calendar.jsx"},{"name":"MonthPicker","sourcePath":"components/structure/Calendar.jsx"},{"name":"Calendar","sourcePath":"components/structure/Calendar.jsx"},{"name":"Timeline","sourcePath":"components/structure/Timeline.jsx"},{"name":"TimelineSkeleton","sourcePath":"components/structure/Timeline.jsx"}],"sourceHashes":{"assets/icons.js":"4c75f639fb0b","components/charts/Chart.jsx":"84002cc68ca4","components/core/Badge.jsx":"c81526f78ada","components/core/Button.jsx":"87eea7b05c0b","components/core/Card.jsx":"64df6695299f","components/core/IconButton.jsx":"92028bfbe305","components/data/CodeBlock.jsx":"77de1ac6852a","components/data/DataTable.jsx":"17cb809ff1a4","components/data/KeyValue.jsx":"85143955d7b5","components/feedback/EmptyState.jsx":"2895af4c5f80","components/feedback/ErrorState.jsx":"0c04c17442bd","components/feedback/Skeleton.jsx":"3d4af79d1b34","components/forms/Checkbox.jsx":"39a4e5b01186","components/forms/Input.jsx":"ba1251ab65bd","components/forms/Select.jsx":"c42ee9f1fa7b","components/forms/Switch.jsx":"ac3b15904694","components/identity/Avatar.jsx":"b6c1ab9e2c8e","components/navigation/Sidebar.jsx":"9d0155c80c18","components/navigation/Tabs.jsx":"8a4df90d9735","components/overlays/Drawer.jsx":"14ba8b76d52a","components/overlays/Modal.jsx":"ee0917713e97","components/overlays/Toast.jsx":"b01b87d9eedb","components/overlays/Tooltip.jsx":"bf196adc89cf","components/structure/Calendar.jsx":"c610a60b4999","components/structure/Timeline.jsx":"b130270ff5be","tailwind.config.js":"52dc5fe06ef6","ui_kits/chat-widget/ChatWidget.jsx":"bf364e58d859","ui_kits/marketing-landing/LandingPage.jsx":"10416ccaaf03","ui_kits/operator-dashboard/OperatorDashboard.jsx":"90a6b3bf10a5","ui_kits/week-calendar/WeekCalendarScreen.jsx":"19299387ea14"},"inlinedExternals":[],"unexposedExports":[]} */

(() => {

const __ds_ns = (window.KontorKanonDesignSystem_452420 = window.KontorKanonDesignSystem_452420 || {});

const __ds_scope = {};

(__ds_ns.__errors = __ds_ns.__errors || []);

// assets/icons.js
try { (() => {
/* Kontor & Kanon icon set — follows Lucide geometry (24×24, 2px stroke, round caps).
   SUBSTITUTION: no brand icons were supplied. Lucide is the canonical set; in
   production use `lucide-react`. These inline mirrors keep the kits self-contained
   and recolor via currentColor. Exposes window.KKIcons. */
(function () {
  const React = window.React;
  const S = (paths, props = {}) => (extra = {}) => React.createElement('svg', {
    width: extra.size || 18,
    height: extra.size || 18,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: extra.strokeWidth || 2,
    strokeLinecap: 'round',
    strokeLinejoin: 'round',
    'aria-hidden': true,
    ...props,
    ...(extra.style ? {
      style: extra.style
    } : {})
  }, paths.map((d, i) => React.createElement('path', {
    key: i,
    d
  })));
  const M = children => (extra = {}) => React.createElement('svg', {
    width: extra.size || 18,
    height: extra.size || 18,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: extra.strokeWidth || 2,
    strokeLinecap: 'round',
    strokeLinejoin: 'round',
    'aria-hidden': true,
    ...(extra.style ? {
      style: extra.style
    } : {})
  }, children);
  const c = (cx, cy, r) => React.createElement('circle', {
    key: 'c' + cx + cy,
    cx,
    cy,
    r
  });
  const p = (d, k) => React.createElement('path', {
    key: k || d,
    d
  });
  const r = (x, y, w, h, rx) => React.createElement('rect', {
    key: 'r' + x + y,
    x,
    y,
    width: w,
    height: h,
    rx
  });
  window.KKIcons = {
    Home: S(['M3 10.5 12 3l9 7.5', 'M5 9.5V20a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9.5']),
    Inbox: S(['M3 12h5l2 3h4l2-3h5', 'M3 12l3-7h12l3 7v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z']),
    Calendar: M([r(3, 4, 18, 17, 2), p('M3 9h18', 'a'), p('M8 2v4', 'b'), p('M16 2v4', 'c')]),
    Activity: S(['M3 12h4l3 8 4-16 3 8h4']),
    BarChart: M([p('M4 20V10', 'a'), p('M10 20V4', 'b'), p('M16 20v-7', 'c'), p('M4 20h16', 'd')]),
    FileText: S(['M14 3v5h5', 'M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z', 'M9 13h6', 'M9 17h6']),
    Message: S(['M21 15a2 2 0 0 1-2 2H8l-4 4V5a2 2 0 0 1 2-2h13a2 2 0 0 1 2 2z']),
    Settings: M([c(12, 12, 3), p('M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-1.8-.3 1.6 1.6 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.6 1.6 0 0 0-1-1.5 1.6 1.6 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0 .3-1.8 1.6 1.6 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.6 1.6 0 0 0 1.5-1 1.6 1.6 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.8.3H9a1.6 1.6 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.6 1.6 0 0 0 1 1.5 1.6 1.6 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.8V9a1.6 1.6 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1z')]),
    Search: M([c(11, 11, 7), p('M21 21l-4.3-4.3', 'a')]),
    Plus: S(['M12 5v14', 'M5 12h14']),
    Filter: S(['M3 4h18l-7 8v6l-4 2v-8z']),
    ChevronRight: S(['M9 6l6 6-6 6']),
    ChevronDown: S(['M6 9l6 6 6-6']),
    Check: S(['M4 12.5 9 17.5 20 6.5']),
    X: S(['M6 6l12 12', 'M18 6 6 18']),
    Refresh: S(['M20 11a8 8 0 1 0-.6 4', 'M20 5v6h-6']),
    Bot: M([r(4, 8, 16, 12, 3), p('M12 8V4', 'a'), c(9, 14, 0.6), c(15, 14, 0.6), p('M2 13v3', 'b'), p('M22 13v3', 'c')]),
    Zap: S(['M13 2 4 14h7l-1 8 9-12h-7z']),
    Shield: S(['M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z', 'M9 12l2 2 4-4']),
    Send: S(['M22 2 11 13', 'M22 2 15 22l-4-9-9-4z']),
    Paperclip: S(['M21 11.5 12.5 20a5 5 0 0 1-7-7l8.5-8.5a3.3 3.3 0 0 1 4.7 4.7L9.9 17.6a1.7 1.7 0 0 1-2.4-2.4l7.8-7.8']),
    ThumbUp: S(['M7 11v9H4a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1z', 'M7 11l4-8a2 2 0 0 1 2 2v4h5a2 2 0 0 1 2 2.3l-1.3 7A2 2 0 0 1 17.7 20H7']),
    ThumbDown: S(['M17 13V4h3a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1z', 'M17 13l-4 8a2 2 0 0 1-2-2v-4H6a2 2 0 0 1-2-2.3l1.3-7A2 2 0 0 1 7.3 4H17']),
    ArrowRight: S(['M5 12h14', 'M13 6l6 6-6 6']),
    Clock: M([c(12, 12, 9), p('M12 7v5l3 2', 'a')]),
    User: M([c(12, 8, 4), p('M5 21v-1a5 5 0 0 1 5-5h4a5 5 0 0 1 5 5v1', 'a')]),
    Bell: S(['M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9', 'M13.7 21a2 2 0 0 1-3.4 0']),
    Sparkles: S(['M12 3l1.8 4.7L18.5 9.5 13.8 11.3 12 16l-1.8-4.7L5.5 9.5l4.7-1.8z', 'M19 15l.7 1.8 1.8.7-1.8.7-.7 1.8-.7-1.8-1.8-.7 1.8-.7z']),
    Database: M([React.createElement('ellipse', {
      key: 'e',
      cx: 12,
      cy: 5,
      rx: 8,
      ry: 3
    }), p('M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5', 'a'), p('M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6', 'b')]),
    Play: S(['M6 4l14 8-14 8z']),
    Users: M([c(9, 8, 4), p('M2 21v-1a5 5 0 0 1 5-5h4a5 5 0 0 1 5 5v1', 'a'), p('M16 4a4 4 0 0 1 0 8', 'b'), p('M22 21v-1a5 5 0 0 0-4-4.9', 'c')]),
    Phone: S(['M22 16.9v3a2 2 0 0 1-2.2 2 19.8 19.8 0 0 1-8.6-3.1 19.5 19.5 0 0 1-6-6A19.8 19.8 0 0 1 2.1 4.2 2 2 0 0 1 4.1 2h3a2 2 0 0 1 2 1.7c.1.9.4 1.9.7 2.8a2 2 0 0 1-.5 2.1L8.1 9.9a16 16 0 0 0 6 6l1.3-1.3a2 2 0 0 1 2.1-.5c.9.3 1.9.6 2.8.7a2 2 0 0 1 1.7 2z']),
    ExternalLink: S(['M15 3h6v6', 'M10 14 21 3', 'M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6']),
    Dot: M([c(12, 12, 3)]),
    Menu: S(['M4 6h16', 'M4 12h16', 'M4 18h16']),
    Globe: M([c(12, 12, 9), p('M3 12h18', 'a'), p('M12 3a15 15 0 0 1 0 18 15 15 0 0 1 0-18', 'b')])
  };
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "assets/icons.js", error: String((e && e.message) || e) }); }

// components/core/Badge.jsx
try { (() => {
const tones = {
  neutral: {
    fg: 'var(--status-neutral-fg)',
    bg: 'var(--status-neutral-bg)',
    bd: 'var(--status-neutral-border)'
  },
  success: {
    fg: 'var(--status-success-fg)',
    bg: 'var(--status-success-bg)',
    bd: 'var(--status-success-border)'
  },
  warning: {
    fg: 'var(--status-warning-fg)',
    bg: 'var(--status-warning-bg)',
    bd: 'var(--status-warning-border)'
  },
  error: {
    fg: 'var(--status-error-fg)',
    bg: 'var(--status-error-bg)',
    bd: 'var(--status-error-border)'
  },
  info: {
    fg: 'var(--status-info-fg)',
    bg: 'var(--status-info-bg)',
    bd: 'var(--status-info-border)'
  }
};

/**
 * Status badge — compact state label. Optional leading dot.
 */
function Badge({
  children,
  tone = 'neutral',
  dot = false,
  size = 'md',
  style
}) {
  const t = tones[tone] || tones.neutral;
  const sz = size === 'sm' ? {
    fontSize: 'var(--fs-micro)',
    padding: '1px 7px',
    height: 18,
    gap: 5
  } : {
    fontSize: 'var(--fs-caption)',
    padding: '2px 9px',
    height: 22,
    gap: 6
  };
  return /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: sz.gap,
      height: sz.height,
      padding: sz.padding,
      background: t.bg,
      color: t.fg,
      border: `1px solid ${t.bd}`,
      borderRadius: 'var(--radius-full)',
      fontSize: sz.fontSize,
      fontWeight: 'var(--fw-medium)',
      letterSpacing: 'var(--ls-wide)',
      lineHeight: 'var(--lh-tight)',
      whiteSpace: 'nowrap',
      ...style
    }
  }, dot && /*#__PURE__*/React.createElement("span", {
    style: {
      width: 6,
      height: 6,
      borderRadius: '50%',
      background: t.fg,
      flexShrink: 0
    }
  }), children);
}
Object.assign(__ds_scope, { Badge });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Badge.jsx", error: String((e && e.message) || e) }); }

// components/core/Button.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const sizes = {
  sm: {
    height: 28,
    padding: '0 10px',
    fontSize: 'var(--fs-body-sm)',
    gap: 6,
    radius: 'var(--radius-sm)'
  },
  md: {
    height: 34,
    padding: '0 14px',
    fontSize: 'var(--fs-body)',
    gap: 7,
    radius: 'var(--radius-md)'
  },
  lg: {
    height: 42,
    padding: '0 20px',
    fontSize: 'var(--fs-body-lg)',
    gap: 8,
    radius: 'var(--radius-md)'
  }
};
const variants = {
  primary: {
    background: 'var(--accent)',
    color: 'var(--text-on-accent)',
    border: '1px solid transparent',
    '--hoverBg': 'var(--accent-hover)',
    '--activeBg': 'var(--accent-active)'
  },
  secondary: {
    background: 'var(--surface-3)',
    color: 'var(--text-primary)',
    border: '1px solid var(--border-default)',
    '--hoverBg': 'var(--surface-hover)',
    '--activeBg': 'var(--surface-2)'
  },
  ghost: {
    background: 'transparent',
    color: 'var(--text-secondary)',
    border: '1px solid transparent',
    '--hoverBg': 'var(--surface-hover)',
    '--activeBg': 'var(--surface-2)'
  },
  danger: {
    background: 'var(--red-600)',
    color: '#fff',
    border: '1px solid transparent',
    '--hoverBg': 'var(--red-500)',
    '--activeBg': '#c23641'
  }
};

/**
 * Primary action control. Precise, engineering-grade — flat fill, 1px border,
 * 150ms ease-out state changes.
 */
function Button({
  children,
  variant = 'primary',
  size = 'md',
  leadingIcon,
  trailingIcon,
  loading = false,
  disabled = false,
  fullWidth = false,
  type = 'button',
  onClick,
  style,
  ...rest
}) {
  const [hover, setHover] = React.useState(false);
  const [active, setActive] = React.useState(false);
  const sz = sizes[size] || sizes.md;
  const v = variants[variant] || variants.primary;
  const isDisabled = disabled || loading;
  const bg = active ? v['--activeBg'] : hover ? v['--hoverBg'] : v.background;
  return /*#__PURE__*/React.createElement("button", _extends({
    type: type,
    disabled: isDisabled,
    onClick: onClick,
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => {
      setHover(false);
      setActive(false);
    },
    onMouseDown: () => setActive(true),
    onMouseUp: () => setActive(false),
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      gap: sz.gap,
      height: sz.height,
      padding: sz.padding,
      width: fullWidth ? '100%' : 'auto',
      fontFamily: 'var(--font-sans)',
      fontSize: sz.fontSize,
      fontWeight: 'var(--fw-medium)',
      letterSpacing: 'var(--ls-normal)',
      lineHeight: 'var(--lh-tight)',
      whiteSpace: 'nowrap',
      background: isDisabled ? 'var(--surface-2)' : bg,
      color: isDisabled ? 'var(--text-disabled)' : v.color,
      border: isDisabled ? '1px solid var(--border-subtle)' : v.border,
      borderRadius: sz.radius,
      cursor: isDisabled ? 'not-allowed' : 'pointer',
      transition: 'var(--transition-colors)',
      userSelect: 'none',
      opacity: loading ? 0.85 : 1,
      ...style
    }
  }, rest), loading && /*#__PURE__*/React.createElement(Spinner, {
    size: size === 'lg' ? 15 : 13
  }), !loading && leadingIcon, children && /*#__PURE__*/React.createElement("span", null, children), !loading && trailingIcon);
}
function Spinner({
  size = 13
}) {
  return /*#__PURE__*/React.createElement("svg", {
    width: size,
    height: size,
    viewBox: "0 0 24 24",
    fill: "none",
    style: {
      animation: 'kk-spin 0.7s linear infinite'
    },
    "aria-hidden": "true"
  }, /*#__PURE__*/React.createElement("circle", {
    cx: "12",
    cy: "12",
    r: "9",
    stroke: "currentColor",
    strokeWidth: "2.5",
    opacity: "0.25"
  }), /*#__PURE__*/React.createElement("path", {
    d: "M21 12a9 9 0 0 0-9-9",
    stroke: "currentColor",
    strokeWidth: "2.5",
    strokeLinecap: "round"
  }), /*#__PURE__*/React.createElement("style", null, '@keyframes kk-spin{to{transform:rotate(360deg)}}'));
}
Object.assign(__ds_scope, { Button });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Button.jsx", error: String((e && e.message) || e) }); }

// components/core/Card.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Surface container. Depth comes from tone + 1px border, not shadow.
 */
function Card({
  children,
  padding = 'md',
  interactive = false,
  style,
  ...rest
}) {
  const [hover, setHover] = React.useState(false);
  const pad = {
    none: 0,
    sm: 'var(--space-3)',
    md: 'var(--space-5)',
    lg: 'var(--space-6)'
  }[padding] ?? 'var(--space-5)';
  return /*#__PURE__*/React.createElement("div", _extends({
    onMouseEnter: () => interactive && setHover(true),
    onMouseLeave: () => interactive && setHover(false),
    style: {
      background: 'var(--surface-1)',
      border: '1px solid var(--border-default)',
      borderColor: hover ? 'var(--border-strong)' : 'var(--border-default)',
      borderRadius: 'var(--radius-lg)',
      padding: pad,
      transition: 'var(--transition-colors)',
      cursor: interactive ? 'pointer' : 'default',
      ...style
    }
  }, rest), children);
}
function CardHeader({
  title,
  subtitle,
  actions,
  style
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'flex-start',
      justifyContent: 'space-between',
      gap: 'var(--space-4)',
      marginBottom: 'var(--space-4)',
      ...style
    }
  }, /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-h4)',
      fontWeight: 'var(--fw-semibold)',
      color: 'var(--text-primary)',
      letterSpacing: 'var(--ls-tight)'
    }
  }, title), subtitle && /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-body-sm)',
      color: 'var(--text-tertiary)',
      marginTop: 2
    }
  }, subtitle)), actions && /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      gap: 'var(--space-2)',
      flexShrink: 0
    }
  }, actions));
}
Object.assign(__ds_scope, { Card, CardHeader });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Card.jsx", error: String((e && e.message) || e) }); }

// components/core/IconButton.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const sizes = {
  sm: 28,
  md: 34,
  lg: 42
};

/**
 * Square icon-only button. Same interaction language as Button.
 */
function IconButton({
  children,
  icon,
  variant = 'ghost',
  size = 'md',
  disabled = false,
  active = false,
  label,
  onClick,
  style,
  ...rest
}) {
  const [hover, setHover] = React.useState(false);
  const dim = sizes[size] || sizes.md;
  const base = {
    ghost: {
      bg: 'transparent',
      color: 'var(--text-secondary)',
      border: '1px solid transparent'
    },
    secondary: {
      bg: 'var(--surface-3)',
      color: 'var(--text-primary)',
      border: '1px solid var(--border-default)'
    },
    primary: {
      bg: 'var(--accent)',
      color: 'var(--text-on-accent)',
      border: '1px solid transparent'
    }
  }[variant] || {
    bg: 'transparent',
    color: 'var(--text-secondary)',
    border: '1px solid transparent'
  };
  return /*#__PURE__*/React.createElement("button", _extends({
    type: "button",
    "aria-label": label,
    disabled: disabled,
    onClick: onClick,
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      width: dim,
      height: dim,
      borderRadius: 'var(--radius-md)',
      background: active ? 'var(--accent-subtle)' : hover && !disabled ? 'var(--surface-hover)' : base.bg,
      color: disabled ? 'var(--text-disabled)' : active ? 'var(--text-accent)' : base.color,
      border: base.border,
      cursor: disabled ? 'not-allowed' : 'pointer',
      transition: 'var(--transition-colors)',
      padding: 0,
      ...style
    }
  }, rest), icon || children);
}
Object.assign(__ds_scope, { IconButton });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/IconButton.jsx", error: String((e && e.message) || e) }); }

// components/data/CodeBlock.jsx
try { (() => {
// Minimal JSON tokenizer for tinting — keys/strings/numbers/booleans/punctuation.
function tint(line) {
  const parts = [];
  const re = /("(?:\\.|[^"\\])*"(?:\s*:)?)|(\b-?\d+\.?\d*\b)|(\btrue\b|\bfalse\b|\bnull\b)|([{}[\],:])/g;
  let last = 0,
    m;
  while (m = re.exec(line)) {
    if (m.index > last) parts.push({
      t: line.slice(last, m.index),
      c: 'var(--text-secondary)'
    });
    if (m[1]) parts.push({
      t: m[1],
      c: m[1].trim().endsWith(':') ? 'var(--viz-1)' : 'var(--viz-3)'
    });else if (m[2]) parts.push({
      t: m[2],
      c: 'var(--viz-4)'
    });else if (m[3]) parts.push({
      t: m[3],
      c: 'var(--viz-5)'
    });else if (m[4]) parts.push({
      t: m[4],
      c: 'var(--text-tertiary)'
    });
    last = re.lastIndex;
  }
  if (last < line.length) parts.push({
    t: line.slice(last),
    c: 'var(--text-secondary)'
  });
  return parts;
}

/**
 * CodeBlock — collapsed-by-default JSON/code viewer for tool-call args and
 * results. Syntax tinting from the --viz-* palette, copy button, line limit.
 */
function CodeBlock({
  code,
  title,
  collapsedLines = 6
}) {
  const [expanded, setExpanded] = React.useState(false);
  const [copied, setCopied] = React.useState(false);
  const text = typeof code === 'string' ? code : JSON.stringify(code, null, 2);
  const lines = text.split('\n');
  const shown = expanded ? lines : lines.slice(0, collapsedLines);
  const truncated = lines.length > collapsedLines;
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1400);
    } catch (e) {}
  };
  return /*#__PURE__*/React.createElement("div", {
    style: {
      background: 'var(--surface-inset)',
      border: '1px solid var(--border-subtle)',
      borderRadius: 'var(--radius-md)',
      overflow: 'hidden'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      padding: '6px 10px',
      borderBottom: '1px solid var(--border-subtle)'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 'var(--fs-micro)',
      letterSpacing: 'var(--ls-caps)',
      textTransform: 'uppercase',
      color: 'var(--text-tertiary)'
    }
  }, title || 'json'), /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: copy,
    style: {
      background: 'none',
      border: 'none',
      color: copied ? 'var(--status-success-fg)' : 'var(--text-tertiary)',
      cursor: 'pointer',
      fontSize: 'var(--fs-caption)',
      fontFamily: 'var(--font-sans)',
      display: 'flex',
      alignItems: 'center',
      gap: 5,
      padding: 2
    }
  }, /*#__PURE__*/React.createElement("svg", {
    width: "12",
    height: "12",
    viewBox: "0 0 24 24",
    fill: "none",
    "aria-hidden": "true"
  }, /*#__PURE__*/React.createElement("rect", {
    x: "8",
    y: "8",
    width: "12",
    height: "12",
    rx: "2",
    stroke: "currentColor",
    strokeWidth: "2"
  }), /*#__PURE__*/React.createElement("path", {
    d: "M4 16V5a1 1 0 0 1 1-1h11",
    stroke: "currentColor",
    strokeWidth: "2"
  })), copied ? 'Copied' : 'Copy')), /*#__PURE__*/React.createElement("pre", {
    style: {
      margin: 0,
      padding: '10px 12px',
      overflowX: 'auto',
      fontFamily: 'var(--font-mono)',
      fontSize: 'var(--fs-caption)',
      lineHeight: 'var(--lh-normal)'
    }
  }, shown.map((l, i) => /*#__PURE__*/React.createElement("div", {
    key: i,
    style: {
      whiteSpace: 'pre'
    }
  }, tint(l).map((p, j) => /*#__PURE__*/React.createElement("span", {
    key: j,
    style: {
      color: p.c
    }
  }, p.t))))), truncated && /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: () => setExpanded(e => !e),
    style: {
      width: '100%',
      textAlign: 'left',
      padding: '7px 12px',
      background: 'var(--surface-2)',
      border: 'none',
      borderTop: '1px solid var(--border-subtle)',
      color: 'var(--text-accent)',
      fontSize: 'var(--fs-caption)',
      fontFamily: 'var(--font-sans)',
      fontWeight: 'var(--fw-medium)',
      cursor: 'pointer'
    }
  }, expanded ? 'Show less' : `Show ${lines.length - collapsedLines} more lines`));
}
Object.assign(__ds_scope, { CodeBlock });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/data/CodeBlock.jsx", error: String((e && e.message) || e) }); }

// components/data/DataTable.jsx
try { (() => {
/**
 * DataTable — dense, Linear-like table. columns: [{key,header,width,align,mono,render,sortable}].
 * Sort is controlled via sortKey/sortDir/onSort, or managed internally if omitted.
 * Rows with onRowClick are keyboard-operable (Enter/Space) and expose role="button".
 */
function DataTable({
  columns = [],
  rows = [],
  rowKey = 'id',
  onRowClick,
  empty,
  dense = false,
  sortKey,
  sortDir,
  onSort,
  style
}) {
  const [intSort, setIntSort] = React.useState({
    key: null,
    dir: 'ascending'
  });
  const activeKey = sortKey !== undefined ? sortKey : intSort.key;
  const activeDir = sortDir !== undefined ? sortDir : intSort.dir;
  const toggleSort = col => {
    if (!col.sortable) return;
    const nextDir = activeKey === col.key && activeDir === 'ascending' ? 'descending' : 'ascending';
    if (onSort) onSort(col.key, nextDir);else setIntSort({
      key: col.key,
      dir: nextDir
    });
  };
  const sortedRows = React.useMemo(() => {
    if (!activeKey || onSort) return rows; // controlled sort: caller already sorted `rows`
    const col = columns.find(c => c.key === activeKey);
    if (!col || !col.sortable) return rows;
    const copy = [...rows];
    copy.sort((a, b) => {
      const av = a[activeKey],
        bv = b[activeKey];
      const cmp = typeof av === 'number' && typeof bv === 'number' ? av - bv : String(av).localeCompare(String(bv));
      return activeDir === 'ascending' ? cmp : -cmp;
    });
    return copy;
  }, [rows, activeKey, activeDir, onSort, columns]);
  const cellPadV = dense ? '7px' : '10px';
  return /*#__PURE__*/React.createElement("div", {
    style: {
      border: '1px solid var(--border-default)',
      borderRadius: 'var(--radius-lg)',
      overflow: 'hidden',
      background: 'var(--surface-1)',
      ...style
    }
  }, /*#__PURE__*/React.createElement("table", {
    style: {
      width: '100%',
      borderCollapse: 'collapse',
      fontFamily: 'var(--font-sans)'
    }
  }, /*#__PURE__*/React.createElement("thead", null, /*#__PURE__*/React.createElement("tr", null, columns.map(c => {
    const isSorted = c.sortable && activeKey === c.key;
    const ariaSort = c.sortable ? isSorted ? activeDir : 'none' : undefined;
    return /*#__PURE__*/React.createElement("th", {
      key: c.key,
      scope: "col",
      "aria-sort": ariaSort,
      style: {
        textAlign: c.align || 'left',
        padding: 0,
        background: 'var(--surface-2)',
        borderBottom: '1px solid var(--border-default)',
        width: c.width,
        position: 'sticky',
        top: 0
      }
    }, c.sortable ? /*#__PURE__*/React.createElement("button", {
      type: "button",
      onClick: () => toggleSort(c),
      style: {
        display: 'inline-flex',
        alignItems: 'center',
        gap: 4,
        width: '100%',
        justifyContent: c.align === 'right' ? 'flex-end' : 'flex-start',
        padding: `${dense ? 8 : 10}px var(--space-4)`,
        background: 'transparent',
        border: 'none',
        cursor: 'pointer',
        fontSize: 'var(--fs-micro)',
        fontWeight: 'var(--fw-medium)',
        letterSpacing: 'var(--ls-caps)',
        textTransform: 'uppercase',
        color: isSorted ? 'var(--text-secondary)' : 'var(--text-tertiary)',
        whiteSpace: 'nowrap'
      }
    }, c.header, /*#__PURE__*/React.createElement("span", {
      "aria-hidden": "true",
      style: {
        fontSize: 'var(--fs-micro)',
        opacity: isSorted ? 1 : 0.35
      }
    }, isSorted && activeDir === 'descending' ? '▼' : '▲')) : /*#__PURE__*/React.createElement("span", {
      style: {
        display: 'block',
        padding: `${dense ? 8 : 10}px var(--space-4)`,
        fontSize: 'var(--fs-micro)',
        fontWeight: 'var(--fw-medium)',
        letterSpacing: 'var(--ls-caps)',
        textTransform: 'uppercase',
        color: 'var(--text-tertiary)',
        whiteSpace: 'nowrap'
      }
    }, c.header));
  }))), /*#__PURE__*/React.createElement("tbody", null, sortedRows.length === 0 && /*#__PURE__*/React.createElement("tr", null, /*#__PURE__*/React.createElement("td", {
    colSpan: columns.length,
    style: {
      padding: 'var(--space-10)',
      textAlign: 'center',
      color: 'var(--text-tertiary)',
      fontSize: 'var(--fs-body-sm)'
    }
  }, empty || 'No rows')), sortedRows.map((row, i) => /*#__PURE__*/React.createElement(Row, {
    key: row[rowKey] ?? i,
    row: row,
    columns: columns,
    last: i === sortedRows.length - 1,
    cellPadV: cellPadV,
    onRowClick: onRowClick
  })))));
}
function Row({
  row,
  columns,
  last,
  cellPadV,
  onRowClick
}) {
  const [hover, setHover] = React.useState(false);
  const interactive = !!onRowClick;
  return /*#__PURE__*/React.createElement("tr", {
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    onClick: interactive ? () => onRowClick(row) : undefined,
    role: interactive ? 'button' : undefined,
    tabIndex: interactive ? 0 : undefined,
    onKeyDown: interactive ? e => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        onRowClick(row);
      }
    } : undefined,
    style: {
      background: hover && interactive ? 'var(--surface-2)' : 'transparent',
      cursor: interactive ? 'pointer' : 'default',
      transition: 'background-color var(--dur-fast) var(--ease-out)',
      outlineOffset: -2
    }
  }, columns.map(c => /*#__PURE__*/React.createElement("td", {
    key: c.key,
    style: {
      textAlign: c.align || 'left',
      padding: `${cellPadV} var(--space-4)`,
      borderBottom: last ? 'none' : '1px solid var(--border-subtle)',
      fontSize: 'var(--fs-body-sm)',
      color: 'var(--text-primary)',
      fontFamily: c.mono ? 'var(--font-mono)' : 'var(--font-sans)',
      whiteSpace: 'nowrap'
    }
  }, c.render ? c.render(row[c.key], row) : row[c.key])));
}
Object.assign(__ds_scope, { DataTable });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/data/DataTable.jsx", error: String((e && e.message) || e) }); }

// components/data/KeyValue.jsx
try { (() => {
/**
 * KeyValue — one label/value row for detail panels. Monospace value for
 * IDs, timestamps and latencies.
 */
function KeyValue({
  label,
  value,
  mono = false,
  align = 'right'
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'baseline',
      justifyContent: 'space-between',
      gap: 12,
      padding: '7px 0'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 'var(--fs-body-sm)',
      color: 'var(--text-tertiary)',
      flexShrink: 0
    }
  }, label), /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 'var(--fs-body-sm)',
      color: 'var(--text-primary)',
      fontFamily: mono ? 'var(--font-mono)' : 'var(--font-sans)',
      textAlign: align,
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      whiteSpace: 'nowrap'
    }
  }, value));
}

/** KeyValueList — a divided stack of KeyValue rows. */
function KeyValueList({
  items = []
}) {
  return /*#__PURE__*/React.createElement("div", null, items.map((it, i) => /*#__PURE__*/React.createElement("div", {
    key: it.label + i,
    style: {
      borderBottom: i === items.length - 1 ? 'none' : '1px solid var(--border-subtle)'
    }
  }, /*#__PURE__*/React.createElement(KeyValue, it))));
}
Object.assign(__ds_scope, { KeyValue, KeyValueList });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/data/KeyValue.jsx", error: String((e && e.message) || e) }); }

// components/feedback/EmptyState.jsx
try { (() => {
/**
 * EmptyState — calm zero-data view with optional action.
 */
function EmptyState({
  icon,
  title,
  description,
  action,
  compact = false,
  style
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      textAlign: 'center',
      padding: compact ? 'var(--space-8) var(--space-6)' : 'var(--space-12) var(--space-6)',
      ...style
    }
  }, icon && /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      width: 44,
      height: 44,
      borderRadius: 'var(--radius-lg)',
      background: 'var(--surface-3)',
      border: '1px solid var(--border-default)',
      color: 'var(--text-tertiary)',
      marginBottom: 'var(--space-4)'
    }
  }, icon), /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-h4)',
      fontWeight: 'var(--fw-semibold)',
      color: 'var(--text-primary)',
      letterSpacing: 'var(--ls-tight)'
    }
  }, title), description && /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-body-sm)',
      color: 'var(--text-tertiary)',
      marginTop: 6,
      maxWidth: 340,
      lineHeight: 'var(--lh-normal)'
    }
  }, description), action && /*#__PURE__*/React.createElement("div", {
    style: {
      marginTop: 'var(--space-5)'
    }
  }, action));
}
Object.assign(__ds_scope, { EmptyState });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/EmptyState.jsx", error: String((e && e.message) || e) }); }

// components/feedback/ErrorState.jsx
try { (() => {
/**
 * ErrorState — failure view with retry. Optional monospace detail (error code / trace id).
 */
function ErrorState({
  title = 'Something went wrong',
  description,
  detail,
  onRetry,
  retryLabel = 'Try again',
  compact = false,
  style
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      textAlign: 'center',
      padding: compact ? 'var(--space-8) var(--space-6)' : 'var(--space-12) var(--space-6)',
      ...style
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      width: 44,
      height: 44,
      borderRadius: 'var(--radius-lg)',
      background: 'var(--status-error-bg)',
      border: '1px solid var(--status-error-border)',
      color: 'var(--status-error-fg)',
      marginBottom: 'var(--space-4)'
    }
  }, /*#__PURE__*/React.createElement("svg", {
    width: "20",
    height: "20",
    viewBox: "0 0 24 24",
    fill: "none",
    "aria-hidden": "true"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M12 8v5M12 16.5v.5",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round"
  }), /*#__PURE__*/React.createElement("path", {
    d: "M10.3 3.9L2.4 18a2 2 0 0 0 1.7 3h15.8a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z",
    stroke: "currentColor",
    strokeWidth: "1.6"
  }))), /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-h4)',
      fontWeight: 'var(--fw-semibold)',
      color: 'var(--text-primary)',
      letterSpacing: 'var(--ls-tight)'
    }
  }, title), description && /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-body-sm)',
      color: 'var(--text-tertiary)',
      marginTop: 6,
      maxWidth: 360,
      lineHeight: 'var(--lh-normal)'
    }
  }, description), detail && /*#__PURE__*/React.createElement("code", {
    style: {
      marginTop: 'var(--space-3)',
      padding: '4px 10px',
      background: 'var(--surface-inset)',
      border: '1px solid var(--border-subtle)',
      borderRadius: 'var(--radius-sm)',
      fontFamily: 'var(--font-mono)',
      fontSize: 'var(--fs-caption)',
      color: 'var(--text-secondary)'
    }
  }, detail), onRetry && /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: onRetry,
    style: {
      marginTop: 'var(--space-5)',
      display: 'inline-flex',
      alignItems: 'center',
      gap: 7,
      height: 34,
      padding: '0 16px',
      background: 'var(--surface-3)',
      color: 'var(--text-primary)',
      border: '1px solid var(--border-default)',
      borderRadius: 'var(--radius-md)',
      fontFamily: 'var(--font-sans)',
      fontSize: 'var(--fs-body)',
      fontWeight: 'var(--fw-medium)',
      cursor: 'pointer'
    }
  }, /*#__PURE__*/React.createElement("svg", {
    width: "14",
    height: "14",
    viewBox: "0 0 24 24",
    fill: "none",
    "aria-hidden": "true"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M20 11a8 8 0 1 0-.6 4M20 5v6h-6",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round",
    strokeLinejoin: "round"
  })), retryLabel));
}
Object.assign(__ds_scope, { ErrorState });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/ErrorState.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Skeleton.jsx
try { (() => {
/**
 * Skeleton — loading placeholder. Shimmer, not spinner.
 * Respects prefers-reduced-motion (falls back to a static tint).
 */
function Skeleton({
  width = '100%',
  height = 14,
  radius = 'var(--radius-sm)',
  circle = false,
  style
}) {
  return /*#__PURE__*/React.createElement("span", {
    "aria-hidden": "true",
    style: {
      display: 'block',
      width,
      height: circle ? width : height,
      borderRadius: circle ? '50%' : radius,
      background: 'linear-gradient(90deg, var(--skeleton-base) 0%, var(--skeleton-shine) 50%, var(--skeleton-base) 100%)',
      backgroundSize: '200% 100%',
      animation: 'kk-shimmer 1.4s ease-in-out infinite',
      ...style
    }
  }, /*#__PURE__*/React.createElement("style", null, '@keyframes kk-shimmer{0%{background-position:200% 0}100%{background-position:-200% 0}}@media (prefers-reduced-motion:reduce){*{animation:none!important}}'));
}

/** Prebuilt skeleton for a text block of N lines. */
function SkeletonText({
  lines = 3,
  gap = 8,
  lastWidth = '60%'
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap
    }
  }, Array.from({
    length: lines
  }).map((_, i) => /*#__PURE__*/React.createElement(Skeleton, {
    key: i,
    height: 12,
    width: i === lines - 1 ? lastWidth : '100%'
  })));
}

/** Prebuilt skeleton row matching DataTable density. */
function SkeletonRow({
  columns = 4
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      gap: 'var(--space-6)',
      padding: '10px var(--space-4)',
      borderBottom: '1px solid var(--border-subtle)'
    }
  }, Array.from({
    length: columns
  }).map((_, i) => /*#__PURE__*/React.createElement(Skeleton, {
    key: i,
    height: 12,
    width: i === 0 ? 90 : `${18 + i * 11 % 40}%`
  })));
}
Object.assign(__ds_scope, { Skeleton, SkeletonText, SkeletonRow });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Skeleton.jsx", error: String((e && e.message) || e) }); }

// components/charts/Chart.jsx
try { (() => {
/**
 * Chart — thin, dependency-free SVG wrapper over the --viz-* tokens.
 * No charting library: plain inline SVG. Covers the three shapes the
 * dashboard needs — Sparkline, BarChart (over time), DonutChart.
 */
function Sparkline({
  data = [],
  tone = 'var(--viz-1)',
  width = 160,
  height = 40,
  loading,
  empty
}) {
  if (loading) return /*#__PURE__*/React.createElement(__ds_scope.Skeleton, {
    width: width,
    height: height
  });
  if (!data.length) return /*#__PURE__*/React.createElement(ChartEmpty, {
    width: width,
    height: height,
    label: empty || 'No data'
  });
  const max = Math.max(...data),
    min = Math.min(...data);
  const span = max - min || 1;
  const pts = data.map((v, i) => [i / (data.length - 1 || 1) * width, height - (v - min) / span * (height - 4) - 2]);
  const path = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' ');
  return /*#__PURE__*/React.createElement("svg", {
    width: width,
    height: height,
    viewBox: `0 0 ${width} ${height}`,
    role: "img",
    "aria-label": "Trend sparkline"
  }, /*#__PURE__*/React.createElement("path", {
    d: path,
    fill: "none",
    stroke: tone,
    strokeWidth: "2",
    strokeLinecap: "round",
    strokeLinejoin: "round"
  }));
}
function BarChart({
  data = [],
  tone = 'var(--viz-1)',
  width = 320,
  height = 180,
  loading,
  empty
}) {
  const [hoverI, setHoverI] = React.useState(null);
  if (loading) return /*#__PURE__*/React.createElement(__ds_scope.Skeleton, {
    width: width,
    height: height
  });
  if (!data.length) return /*#__PURE__*/React.createElement(ChartEmpty, {
    width: width,
    height: height,
    label: empty || 'No data'
  });
  const max = Math.max(...data.map(d => d.value), 1);
  const padL = 28,
    padB = 20,
    plotW = width - padL,
    plotH = height - padB;
  const bw = plotW / data.length;
  const gridLines = [0, 0.25, 0.5, 0.75, 1];
  return /*#__PURE__*/React.createElement("svg", {
    width: width,
    height: height,
    viewBox: `0 0 ${width} ${height}`,
    role: "img",
    "aria-label": "Bar chart over time"
  }, gridLines.map((g, i) => /*#__PURE__*/React.createElement("line", {
    key: i,
    x1: padL,
    x2: width,
    y1: plotH - g * plotH,
    y2: plotH - g * plotH,
    stroke: "var(--border-subtle)",
    strokeWidth: "1"
  })), gridLines.map((g, i) => /*#__PURE__*/React.createElement("text", {
    key: i,
    x: 0,
    y: plotH - g * plotH + 3,
    fontFamily: "var(--font-mono)",
    fontSize: "9",
    fill: "var(--text-tertiary)"
  }, Math.round(g * max))), data.map((d, i) => {
    const h = d.value / max * plotH;
    return /*#__PURE__*/React.createElement("g", {
      key: i,
      onMouseEnter: () => setHoverI(i),
      onMouseLeave: () => setHoverI(null)
    }, /*#__PURE__*/React.createElement("rect", {
      x: padL + i * bw + bw * 0.15,
      y: plotH - h,
      width: bw * 0.7,
      height: h,
      rx: "2",
      fill: hoverI === i ? tone : tone,
      opacity: hoverI === null || hoverI === i ? 1 : 0.55
    }), /*#__PURE__*/React.createElement("text", {
      x: padL + i * bw + bw / 2,
      y: height - 5,
      textAnchor: "middle",
      fontFamily: "var(--font-mono)",
      fontSize: "9",
      fill: "var(--text-tertiary)"
    }, d.label), hoverI === i && /*#__PURE__*/React.createElement("g", null, /*#__PURE__*/React.createElement("rect", {
      x: padL + i * bw + bw / 2 - 16,
      y: plotH - h - 20,
      width: "32",
      height: "16",
      rx: "4",
      fill: "var(--surface-3)",
      stroke: "var(--border-default)"
    }), /*#__PURE__*/React.createElement("text", {
      x: padL + i * bw + bw / 2,
      y: plotH - h - 9,
      textAnchor: "middle",
      fontFamily: "var(--font-mono)",
      fontSize: "9",
      fill: "var(--text-primary)"
    }, d.value)));
  }));
}
function DonutChart({
  data = [],
  size = 140,
  thickness = 16,
  loading,
  empty
}) {
  if (loading) return /*#__PURE__*/React.createElement(__ds_scope.Skeleton, {
    width: size,
    height: size,
    circle: true
  });
  const total = data.reduce((s, d) => s + d.value, 0);
  if (!data.length || total === 0) return /*#__PURE__*/React.createElement(ChartEmpty, {
    width: size,
    height: size,
    label: empty || 'No data',
    round: true
  });
  const r = (size - thickness) / 2,
    c = size / 2,
    circumf = 2 * Math.PI * r;
  let acc = 0;
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 16
    }
  }, /*#__PURE__*/React.createElement("svg", {
    width: size,
    height: size,
    viewBox: `0 0 ${size} ${size}`,
    role: "img",
    "aria-label": "Donut chart"
  }, /*#__PURE__*/React.createElement("circle", {
    cx: c,
    cy: c,
    r: r,
    fill: "none",
    stroke: "var(--surface-3)",
    strokeWidth: thickness
  }), data.map((d, i) => {
    const frac = d.value / total;
    const dash = frac * circumf;
    const el = /*#__PURE__*/React.createElement("circle", {
      key: i,
      cx: c,
      cy: c,
      r: r,
      fill: "none",
      stroke: d.tone || `var(--viz-${i % 6 + 1})`,
      strokeWidth: thickness,
      strokeDasharray: `${dash} ${circumf - dash}`,
      strokeDashoffset: -acc,
      transform: `rotate(-90 ${c} ${c})`,
      strokeLinecap: "butt"
    });
    acc += dash;
    return el;
  }), /*#__PURE__*/React.createElement("text", {
    x: c,
    y: c - 2,
    textAnchor: "middle",
    fontFamily: "var(--font-mono)",
    fontSize: "18",
    fontWeight: "600",
    fill: "var(--text-primary)"
  }, total), /*#__PURE__*/React.createElement("text", {
    x: c,
    y: c + 14,
    textAnchor: "middle",
    fontFamily: "var(--font-sans)",
    fontSize: "9",
    fill: "var(--text-tertiary)"
  }, "total")), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 6
    }
  }, data.map((d, i) => /*#__PURE__*/React.createElement("div", {
    key: i,
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 6,
      fontSize: 'var(--fs-caption)',
      color: 'var(--text-secondary)'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      width: 8,
      height: 8,
      borderRadius: 2,
      background: d.tone || `var(--viz-${i % 6 + 1})`,
      flexShrink: 0
    }
  }), d.label, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      color: 'var(--text-tertiary)'
    }
  }, d.value)))));
}
function ChartEmpty({
  width,
  height,
  label,
  round
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      width,
      height,
      borderRadius: round ? '50%' : 'var(--radius-md)',
      border: '1px dashed var(--border-default)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      color: 'var(--text-tertiary)',
      fontSize: 'var(--fs-caption)',
      textAlign: 'center',
      padding: 8
    }
  }, label);
}

/** Chart — namespace export bundling the three shapes (Chart.Sparkline, Chart.BarChart, Chart.DonutChart). */
const Chart = {
  Sparkline,
  BarChart,
  DonutChart
};
Object.assign(__ds_scope, { Sparkline, BarChart, DonutChart, Chart });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/charts/Chart.jsx", error: String((e && e.message) || e) }); }

// components/forms/Checkbox.jsx
try { (() => {
/**
 * Checkbox with label. Accent fill when checked.
 */
function Checkbox({
  label,
  description,
  checked,
  defaultChecked,
  disabled = false,
  onChange,
  id,
  style
}) {
  const reactId = React.useId();
  const cbId = id || reactId;
  const [internal, setInternal] = React.useState(defaultChecked || false);
  const isControlled = checked !== undefined;
  const on = isControlled ? checked : internal;
  const toggle = e => {
    if (disabled) return;
    if (!isControlled) setInternal(e.target.checked);
    onChange && onChange(e);
  };
  return /*#__PURE__*/React.createElement("label", {
    htmlFor: cbId,
    style: {
      display: 'flex',
      gap: 10,
      alignItems: description ? 'flex-start' : 'center',
      cursor: disabled ? 'not-allowed' : 'pointer',
      opacity: disabled ? 0.55 : 1,
      ...style
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      position: 'relative',
      flexShrink: 0,
      width: 18,
      height: 18,
      marginTop: description ? 1 : 0,
      borderRadius: 'var(--radius-xs)',
      border: `1px solid ${on ? 'var(--accent)' : 'var(--border-strong)'}`,
      background: on ? 'var(--accent)' : 'var(--surface-inset)',
      transition: 'var(--transition-colors)',
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center'
    }
  }, /*#__PURE__*/React.createElement("input", {
    id: cbId,
    type: "checkbox",
    checked: on,
    disabled: disabled,
    onChange: toggle,
    style: {
      position: 'absolute',
      opacity: 0,
      width: '100%',
      height: '100%',
      margin: 0,
      cursor: 'inherit'
    }
  }), on && /*#__PURE__*/React.createElement("svg", {
    width: "12",
    height: "12",
    viewBox: "0 0 24 24",
    fill: "none",
    "aria-hidden": "true"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M5 12.5l4.5 4.5L19 7",
    stroke: "#fff",
    strokeWidth: "2.6",
    strokeLinecap: "round",
    strokeLinejoin: "round"
  }))), (label || description) && /*#__PURE__*/React.createElement("span", null, label && /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 'var(--fs-body)',
      color: 'var(--text-primary)',
      display: 'block'
    }
  }, label), description && /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 'var(--fs-body-sm)',
      color: 'var(--text-tertiary)',
      display: 'block',
      marginTop: 1
    }
  }, description)));
}
Object.assign(__ds_scope, { Checkbox });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Checkbox.jsx", error: String((e && e.message) || e) }); }

// components/forms/Input.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Text input. 1px border, accent focus ring, monospace-friendly.
 */
function Input({
  label,
  hint,
  error,
  leadingIcon,
  trailingIcon,
  size = 'md',
  mono = false,
  disabled = false,
  id,
  style,
  containerStyle,
  ...rest
}) {
  const [focus, setFocus] = React.useState(false);
  const reactId = React.useId();
  const inputId = id || reactId;
  const h = size === 'sm' ? 30 : size === 'lg' ? 40 : 34;
  const borderColor = error ? 'var(--status-error-border)' : focus ? 'var(--border-focus)' : 'var(--border-default)';
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 6,
      ...containerStyle
    }
  }, label && /*#__PURE__*/React.createElement("label", {
    htmlFor: inputId,
    style: {
      fontSize: 'var(--fs-body-sm)',
      fontWeight: 'var(--fw-medium)',
      color: 'var(--text-secondary)'
    }
  }, label), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 8,
      height: h,
      padding: '0 10px',
      background: disabled ? 'var(--surface-2)' : 'var(--surface-inset)',
      border: `1px solid ${borderColor}`,
      borderRadius: 'var(--radius-md)',
      boxShadow: focus && !error ? '0 0 0 3px var(--focus-ring)' : 'none',
      transition: 'var(--transition-colors), box-shadow var(--dur-fast) var(--ease-out)'
    }
  }, leadingIcon && /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--text-tertiary)',
      display: 'flex',
      flexShrink: 0
    }
  }, leadingIcon), /*#__PURE__*/React.createElement("input", _extends({
    id: inputId,
    disabled: disabled,
    onFocus: () => setFocus(true),
    onBlur: () => setFocus(false),
    style: {
      flex: 1,
      minWidth: 0,
      height: '100%',
      border: 'none',
      outline: 'none',
      background: 'transparent',
      color: disabled ? 'var(--text-disabled)' : 'var(--text-primary)',
      fontFamily: mono ? 'var(--font-mono)' : 'var(--font-sans)',
      fontSize: size === 'sm' ? 'var(--fs-body-sm)' : 'var(--fs-body)',
      ...style
    }
  }, rest)), trailingIcon && /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--text-tertiary)',
      display: 'flex',
      flexShrink: 0
    }
  }, trailingIcon)), (hint || error) && /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 'var(--fs-caption)',
      color: error ? 'var(--status-error-fg)' : 'var(--text-tertiary)'
    }
  }, error || hint));
}
Object.assign(__ds_scope, { Input });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Input.jsx", error: String((e && e.message) || e) }); }

// components/forms/Select.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Native select styled to match Input.
 */
function Select({
  label,
  hint,
  error,
  options = [],
  size = 'md',
  disabled = false,
  id,
  value,
  onChange,
  style,
  containerStyle,
  ...rest
}) {
  const [focus, setFocus] = React.useState(false);
  const reactId = React.useId();
  const selId = id || reactId;
  const h = size === 'sm' ? 30 : size === 'lg' ? 40 : 34;
  const borderColor = error ? 'var(--status-error-border)' : focus ? 'var(--border-focus)' : 'var(--border-default)';
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 6,
      ...containerStyle
    }
  }, label && /*#__PURE__*/React.createElement("label", {
    htmlFor: selId,
    style: {
      fontSize: 'var(--fs-body-sm)',
      fontWeight: 'var(--fw-medium)',
      color: 'var(--text-secondary)'
    }
  }, label), /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'relative',
      display: 'flex',
      alignItems: 'center'
    }
  }, /*#__PURE__*/React.createElement("select", _extends({
    id: selId,
    disabled: disabled,
    value: value,
    onChange: onChange,
    onFocus: () => setFocus(true),
    onBlur: () => setFocus(false),
    style: {
      appearance: 'none',
      WebkitAppearance: 'none',
      width: '100%',
      height: h,
      padding: '0 32px 0 10px',
      background: disabled ? 'var(--surface-2)' : 'var(--surface-inset)',
      color: disabled ? 'var(--text-disabled)' : 'var(--text-primary)',
      border: `1px solid ${borderColor}`,
      borderRadius: 'var(--radius-md)',
      outline: 'none',
      boxShadow: focus && !error ? '0 0 0 3px var(--focus-ring)' : 'none',
      fontFamily: 'var(--font-sans)',
      fontSize: size === 'sm' ? 'var(--fs-body-sm)' : 'var(--fs-body)',
      cursor: disabled ? 'not-allowed' : 'pointer',
      transition: 'var(--transition-colors), box-shadow var(--dur-fast) var(--ease-out)',
      ...style
    }
  }, rest), options.map(o => {
    const opt = typeof o === 'string' ? {
      value: o,
      label: o
    } : o;
    return /*#__PURE__*/React.createElement("option", {
      key: opt.value,
      value: opt.value
    }, opt.label);
  })), /*#__PURE__*/React.createElement("svg", {
    width: "14",
    height: "14",
    viewBox: "0 0 24 24",
    fill: "none",
    style: {
      position: 'absolute',
      right: 10,
      pointerEvents: 'none',
      color: 'var(--text-tertiary)'
    },
    "aria-hidden": "true"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M6 9l6 6 6-6",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round",
    strokeLinejoin: "round"
  }))), (hint || error) && /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 'var(--fs-caption)',
      color: error ? 'var(--status-error-fg)' : 'var(--text-tertiary)'
    }
  }, error || hint));
}
Object.assign(__ds_scope, { Select });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Select.jsx", error: String((e && e.message) || e) }); }

// components/forms/Switch.jsx
try { (() => {
/**
 * Switch — for immediate on/off settings (not form submit).
 */
function Switch({
  label,
  description,
  checked,
  defaultChecked,
  disabled = false,
  onChange,
  id,
  style
}) {
  const reactId = React.useId();
  const swId = id || reactId;
  const [internal, setInternal] = React.useState(defaultChecked || false);
  const isControlled = checked !== undefined;
  const on = isControlled ? checked : internal;
  const toggle = e => {
    if (disabled) return;
    if (!isControlled) setInternal(e.target.checked);
    onChange && onChange(e);
  };
  return /*#__PURE__*/React.createElement("label", {
    htmlFor: swId,
    style: {
      display: 'flex',
      gap: 10,
      alignItems: description ? 'flex-start' : 'center',
      cursor: disabled ? 'not-allowed' : 'pointer',
      opacity: disabled ? 0.55 : 1,
      ...style
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      position: 'relative',
      flexShrink: 0,
      width: 34,
      height: 20,
      marginTop: description ? 1 : 0,
      borderRadius: 'var(--radius-full)',
      background: on ? 'var(--accent)' : 'var(--surface-3)',
      border: `1px solid ${on ? 'var(--accent)' : 'var(--border-strong)'}`,
      transition: 'var(--transition-colors)'
    }
  }, /*#__PURE__*/React.createElement("input", {
    id: swId,
    type: "checkbox",
    checked: on,
    disabled: disabled,
    onChange: toggle,
    style: {
      position: 'absolute',
      opacity: 0,
      width: '100%',
      height: '100%',
      margin: 0,
      cursor: 'inherit'
    }
  }), /*#__PURE__*/React.createElement("span", {
    style: {
      position: 'absolute',
      top: 2,
      left: on ? 16 : 2,
      width: 14,
      height: 14,
      borderRadius: '50%',
      background: '#fff',
      transition: 'left var(--dur-base) var(--ease-out)',
      boxShadow: '0 1px 2px rgba(0,0,0,0.3)'
    }
  })), (label || description) && /*#__PURE__*/React.createElement("span", null, label && /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 'var(--fs-body)',
      color: 'var(--text-primary)',
      display: 'block'
    }
  }, label), description && /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 'var(--fs-body-sm)',
      color: 'var(--text-tertiary)',
      display: 'block',
      marginTop: 1
    }
  }, description)));
}
Object.assign(__ds_scope, { Switch });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Switch.jsx", error: String((e && e.message) || e) }); }

// components/identity/Avatar.jsx
try { (() => {
const sizes = {
  sm: 24,
  md: 32,
  lg: 40
};
const fontSizes = {
  sm: 'var(--fs-micro)',
  md: 'var(--fs-caption)',
  lg: 'var(--fs-body-sm)'
};
function initials(name = '') {
  const parts = name.trim().split(/\s+/);
  return ((parts[0]?.[0] || '') + (parts[1]?.[0] || '')).toUpperCase();
}
const statusColor = {
  online: 'var(--status-success-fg)',
  busy: 'var(--status-error-fg)',
  away: 'var(--status-warning-fg)',
  offline: 'var(--text-tertiary)'
};

/**
 * Avatar — image with initials fallback. Optional status dot.
 */
function Avatar({
  name,
  src,
  size = 'md',
  status,
  style
}) {
  const dim = sizes[size] || sizes.md;
  return /*#__PURE__*/React.createElement("span", {
    style: {
      position: 'relative',
      display: 'inline-flex',
      flexShrink: 0,
      width: dim,
      height: dim,
      ...style
    }
  }, src ? /*#__PURE__*/React.createElement("img", {
    src: src,
    alt: name || '',
    width: dim,
    height: dim,
    style: {
      borderRadius: '50%',
      objectFit: 'cover',
      border: '1px solid var(--border-default)'
    }
  }) : /*#__PURE__*/React.createElement("span", {
    title: name,
    style: {
      width: dim,
      height: dim,
      borderRadius: '50%',
      background: 'var(--surface-3)',
      border: '1px solid var(--border-default)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      color: 'var(--text-secondary)',
      fontSize: fontSizes[size] || fontSizes.md,
      fontWeight: 'var(--fw-medium)',
      letterSpacing: 'var(--ls-normal)'
    }
  }, initials(name)), status && /*#__PURE__*/React.createElement("span", {
    "aria-hidden": "true",
    style: {
      position: 'absolute',
      right: -1,
      bottom: -1,
      width: Math.max(8, dim * 0.28),
      height: Math.max(8, dim * 0.28),
      borderRadius: '50%',
      background: statusColor[status] || statusColor.offline,
      border: '2px solid var(--surface-1)'
    }
  }));
}

/**
 * AvatarGroup — overlapping stack for staff assignment; overflow shows "+N".
 */
function AvatarGroup({
  items = [],
  max = 4,
  size = 'sm'
}) {
  const dim = sizes[size] || sizes.sm;
  const visible = items.slice(0, max);
  const overflow = items.length - visible.length;
  return /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'inline-flex',
      alignItems: 'center'
    }
  }, visible.map((it, i) => /*#__PURE__*/React.createElement("span", {
    key: it.id ?? i,
    style: {
      marginLeft: i === 0 ? 0 : -Math.round(dim * 0.28),
      position: 'relative',
      zIndex: visible.length - i
    }
  }, /*#__PURE__*/React.createElement(Avatar, {
    name: it.name,
    src: it.src,
    size: size,
    style: {
      boxShadow: '0 0 0 2px var(--surface-1)',
      borderRadius: '50%'
    }
  }))), overflow > 0 && /*#__PURE__*/React.createElement("span", {
    style: {
      marginLeft: -Math.round(dim * 0.28),
      width: dim,
      height: dim,
      borderRadius: '50%',
      background: 'var(--surface-3)',
      border: '1px solid var(--border-default)',
      boxShadow: '0 0 0 2px var(--surface-1)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      color: 'var(--text-tertiary)',
      fontSize: fontSizes[size] || fontSizes.sm,
      fontWeight: 'var(--fw-medium)'
    }
  }, "+", overflow));
}
Object.assign(__ds_scope, { Avatar, AvatarGroup });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/identity/Avatar.jsx", error: String((e && e.message) || e) }); }

// components/navigation/Sidebar.jsx
try { (() => {
/**
 * Sidebar — app navigation shell. Composes SidebarItem + SidebarSection.
 */
function Sidebar({
  brand,
  children,
  footer,
  width = 'var(--sidebar-width)',
  style
}) {
  return /*#__PURE__*/React.createElement("aside", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      width,
      flexShrink: 0,
      height: '100%',
      background: 'var(--surface-1)',
      borderRight: '1px solid var(--border-subtle)',
      ...style
    }
  }, brand && /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      height: 'var(--topbar-height)',
      padding: '0 var(--space-4)',
      borderBottom: '1px solid var(--border-subtle)',
      flexShrink: 0
    }
  }, brand), /*#__PURE__*/React.createElement("nav", {
    style: {
      flex: 1,
      overflowY: 'auto',
      padding: 'var(--space-3) var(--space-2)'
    }
  }, children), footer && /*#__PURE__*/React.createElement("div", {
    style: {
      padding: 'var(--space-3) var(--space-2)',
      borderTop: '1px solid var(--border-subtle)'
    }
  }, footer));
}
function SidebarSection({
  label,
  children
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      marginBottom: 'var(--space-4)'
    }
  }, label && /*#__PURE__*/React.createElement("div", {
    style: {
      padding: '0 var(--space-3)',
      marginBottom: 'var(--space-1-5)',
      fontSize: 'var(--fs-micro)',
      fontWeight: 'var(--fw-medium)',
      letterSpacing: 'var(--ls-caps)',
      textTransform: 'uppercase',
      color: 'var(--text-tertiary)'
    }
  }, label), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 1
    }
  }, children));
}
function SidebarItem({
  icon,
  children,
  active = false,
  badge,
  onClick
}) {
  const [hover, setHover] = React.useState(false);
  return /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: onClick,
    "aria-current": active ? 'page' : undefined,
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 9,
      width: '100%',
      height: 32,
      padding: '0 var(--space-3)',
      background: active ? 'var(--accent-subtle)' : hover ? 'var(--surface-hover)' : 'transparent',
      color: active ? 'var(--text-primary)' : 'var(--text-secondary)',
      border: 'none',
      borderRadius: 'var(--radius-sm)',
      fontFamily: 'var(--font-sans)',
      fontSize: 'var(--fs-body)',
      fontWeight: active ? 'var(--fw-medium)' : 'var(--fw-regular)',
      cursor: 'pointer',
      textAlign: 'left',
      transition: 'var(--transition-colors)'
    }
  }, icon && /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'flex',
      flexShrink: 0,
      color: active ? 'var(--text-accent)' : 'inherit'
    }
  }, icon), /*#__PURE__*/React.createElement("span", {
    style: {
      flex: 1,
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      whiteSpace: 'nowrap'
    }
  }, children), badge != null && /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 'var(--fs-micro)',
      fontFamily: 'var(--font-mono)',
      color: 'var(--text-tertiary)'
    }
  }, badge));
}
Object.assign(__ds_scope, { Sidebar, SidebarSection, SidebarItem });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/navigation/Sidebar.jsx", error: String((e && e.message) || e) }); }

// components/navigation/Tabs.jsx
try { (() => {
/**
 * Tabs — underline style, calm 200ms indicator. Full tablist/tab ARIA pattern
 * with roving tabindex + arrow-key navigation (Home/End jump to first/last).
 * Renders only the tab strip — pair each tab with a panel using
 * id={`panel-${value}`} aria-labelledby={`tab-${value}`} role="tabpanel".
 */
function Tabs({
  tabs = [],
  value,
  defaultValue,
  onChange,
  label,
  style
}) {
  const [internal, setInternal] = React.useState(defaultValue ?? (tabs[0] && (tabs[0].value ?? tabs[0])));
  const active = value !== undefined ? value : internal;
  const listRef = React.useRef(null);
  const select = v => {
    if (value === undefined) setInternal(v);
    onChange && onChange(v);
  };
  const norm = tabs.map(t => typeof t === 'string' ? {
    value: t,
    label: t
  } : t);
  const focusTabAt = idx => {
    const btns = listRef.current ? Array.from(listRef.current.querySelectorAll('[role="tab"]')) : [];
    const btn = btns[idx];
    if (btn) btn.focus();
  };
  const onKeyDown = e => {
    const i = norm.findIndex(t => t.value === active);
    if (i === -1) return;
    let next = null;
    if (e.key === 'ArrowRight') next = (i + 1) % norm.length;else if (e.key === 'ArrowLeft') next = (i - 1 + norm.length) % norm.length;else if (e.key === 'Home') next = 0;else if (e.key === 'End') next = norm.length - 1;
    if (next !== null) {
      e.preventDefault();
      select(norm[next].value);
      focusTabAt(next);
    }
  };
  return /*#__PURE__*/React.createElement("div", {
    ref: listRef,
    role: "tablist",
    "aria-label": label,
    onKeyDown: onKeyDown,
    style: {
      display: 'flex',
      gap: 'var(--space-1)',
      borderBottom: '1px solid var(--border-subtle)',
      ...style
    }
  }, norm.map(tab => /*#__PURE__*/React.createElement(TabButton, {
    key: tab.value,
    tab: tab,
    isActive: tab.value === active,
    onSelect: () => select(tab.value)
  })));
}
function TabButton({
  tab,
  isActive,
  onSelect
}) {
  const [hover, setHover] = React.useState(false);
  return /*#__PURE__*/React.createElement("button", {
    type: "button",
    role: "tab",
    id: `tab-${tab.value}`,
    "aria-selected": isActive,
    "aria-controls": `panel-${tab.value}`,
    tabIndex: isActive ? 0 : -1,
    onClick: onSelect,
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    style: {
      position: 'relative',
      display: 'inline-flex',
      alignItems: 'center',
      gap: 6,
      padding: '0 var(--space-3) 10px',
      height: 34,
      background: 'transparent',
      border: 'none',
      color: isActive ? 'var(--text-primary)' : hover ? 'var(--text-secondary)' : 'var(--text-tertiary)',
      fontFamily: 'var(--font-sans)',
      fontSize: 'var(--fs-body)',
      fontWeight: isActive ? 'var(--fw-medium)' : 'var(--fw-regular)',
      cursor: 'pointer',
      transition: 'var(--transition-colors)'
    }
  }, tab.icon && /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'flex'
    }
  }, tab.icon), tab.label, tab.count != null && /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 'var(--fs-caption)',
      fontFamily: 'var(--font-mono)',
      color: 'var(--text-tertiary)',
      marginLeft: 2
    }
  }, tab.count), /*#__PURE__*/React.createElement("span", {
    style: {
      position: 'absolute',
      left: 0,
      right: 0,
      bottom: -1,
      height: 2,
      background: isActive ? 'var(--accent)' : 'transparent',
      borderRadius: '2px 2px 0 0',
      transition: 'var(--transition-colors)'
    }
  }));
}
Object.assign(__ds_scope, { Tabs });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/navigation/Tabs.jsx", error: String((e && e.message) || e) }); }

// components/overlays/Drawer.jsx
try { (() => {
const sizes = {
  sm: 320,
  md: 400,
  lg: 520
};
function CloseBtn({
  onClick
}) {
  const [h, setH] = React.useState(false);
  return /*#__PURE__*/React.createElement("button", {
    type: "button",
    "aria-label": "Close",
    onClick: onClick,
    onMouseEnter: () => setH(true),
    onMouseLeave: () => setH(false),
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      width: 30,
      height: 30,
      flexShrink: 0,
      background: h ? 'var(--surface-hover)' : 'transparent',
      border: 'none',
      borderRadius: 'var(--radius-md)',
      color: 'var(--text-secondary)',
      cursor: 'pointer',
      transition: 'var(--transition-colors)'
    }
  }, /*#__PURE__*/React.createElement("svg", {
    width: "15",
    height: "15",
    viewBox: "0 0 24 24",
    fill: "none",
    "aria-hidden": "true"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M6 6l12 12M18 6 6 18",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round"
  })));
}

/**
 * Drawer — right-side (or left) panel for detail views like the agent trace.
 * Full dialog semantics: focus trap, Escape to close, focus returns to trigger.
 */
function Drawer({
  open,
  onClose,
  title,
  size = 'md',
  side = 'right',
  children,
  footer
}) {
  const panelRef = React.useRef(null);
  const triggerRef = React.useRef(null);
  const titleId = React.useId();
  React.useEffect(() => {
    if (!open) return;
    triggerRef.current = document.activeElement;
    const panel = panelRef.current;
    const focusableSel = 'button:not([disabled]), [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';
    const first = panel && panel.querySelector(focusableSel);
    if (first) first.focus();
    const onKey = e => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose && onClose();
        return;
      }
      if (e.key === 'Tab' && panel) {
        const f = Array.from(panel.querySelectorAll(focusableSel));
        if (!f.length) return;
        const firstEl = f[0],
          lastEl = f[f.length - 1];
        if (e.shiftKey && document.activeElement === firstEl) {
          e.preventDefault();
          lastEl.focus();
        } else if (!e.shiftKey && document.activeElement === lastEl) {
          e.preventDefault();
          firstEl.focus();
        }
      }
    };
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('keydown', onKey);
      if (triggerRef.current && triggerRef.current.focus) triggerRef.current.focus();
    };
  }, [open, onClose]);
  if (!open) return null;
  return /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'fixed',
      inset: 0,
      zIndex: 50,
      display: 'flex',
      justifyContent: side === 'right' ? 'flex-end' : 'flex-start'
    }
  }, /*#__PURE__*/React.createElement("div", {
    onClick: onClose,
    "aria-hidden": "true",
    style: {
      position: 'absolute',
      inset: 0,
      background: 'rgba(4,6,10,0.5)'
    }
  }), /*#__PURE__*/React.createElement("div", {
    ref: panelRef,
    role: "dialog",
    "aria-modal": "true",
    "aria-labelledby": titleId,
    style: {
      position: 'relative',
      width: sizes[size] || sizes.md,
      maxWidth: '92vw',
      height: '100%',
      background: 'var(--surface-1)',
      borderLeft: side === 'right' ? '1px solid var(--border-default)' : 'none',
      borderRight: side === 'left' ? '1px solid var(--border-default)' : 'none',
      boxShadow: 'var(--shadow-overlay)',
      display: 'flex',
      flexDirection: 'column'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      height: 'var(--topbar-height)',
      padding: '0 16px',
      borderBottom: '1px solid var(--border-subtle)',
      flexShrink: 0
    }
  }, /*#__PURE__*/React.createElement("span", {
    id: titleId,
    style: {
      fontWeight: 'var(--fw-semibold)',
      fontSize: 'var(--fs-body)',
      letterSpacing: 'var(--ls-normal)',
      color: 'var(--text-primary)'
    }
  }, title), /*#__PURE__*/React.createElement(CloseBtn, {
    onClick: onClose
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      overflowY: 'auto',
      padding: 16
    }
  }, children), footer && /*#__PURE__*/React.createElement("div", {
    style: {
      padding: 12,
      borderTop: '1px solid var(--border-subtle)',
      flexShrink: 0,
      display: 'flex',
      justifyContent: 'flex-end',
      gap: 8
    }
  }, footer)));
}
Object.assign(__ds_scope, { Drawer });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/overlays/Drawer.jsx", error: String((e && e.message) || e) }); }

// components/overlays/Modal.jsx
try { (() => {
function CloseBtn({
  onClick
}) {
  const [h, setH] = React.useState(false);
  return /*#__PURE__*/React.createElement("button", {
    type: "button",
    "aria-label": "Close",
    onClick: onClick,
    onMouseEnter: () => setH(true),
    onMouseLeave: () => setH(false),
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      width: 30,
      height: 30,
      flexShrink: 0,
      background: h ? 'var(--surface-hover)' : 'transparent',
      border: 'none',
      borderRadius: 'var(--radius-md)',
      color: 'var(--text-secondary)',
      cursor: 'pointer',
      transition: 'var(--transition-colors)'
    }
  }, /*#__PURE__*/React.createElement("svg", {
    width: "15",
    height: "15",
    viewBox: "0 0 24 24",
    fill: "none",
    "aria-hidden": "true"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M6 6l12 12M18 6 6 18",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round"
  })));
}

/**
 * Modal — centered dialog for confirmations. `tone="destructive"` swaps the
 * primary action to the danger button variant's colors.
 */
function Modal({
  open,
  onClose,
  title,
  description,
  tone = 'default',
  children,
  primaryLabel,
  onPrimary,
  secondaryLabel = 'Cancel',
  onSecondary
}) {
  const panelRef = React.useRef(null);
  const triggerRef = React.useRef(null);
  const titleId = React.useId();
  React.useEffect(() => {
    if (!open) return;
    triggerRef.current = document.activeElement;
    const panel = panelRef.current;
    const focusableSel = 'button:not([disabled]), [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';
    const first = panel && panel.querySelector(focusableSel);
    if (first) first.focus();
    const onKey = e => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose && onClose();
        return;
      }
      if (e.key === 'Tab' && panel) {
        const f = Array.from(panel.querySelectorAll(focusableSel));
        if (!f.length) return;
        const firstEl = f[0],
          lastEl = f[f.length - 1];
        if (e.shiftKey && document.activeElement === firstEl) {
          e.preventDefault();
          lastEl.focus();
        } else if (!e.shiftKey && document.activeElement === lastEl) {
          e.preventDefault();
          firstEl.focus();
        }
      }
    };
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('keydown', onKey);
      if (triggerRef.current && triggerRef.current.focus) triggerRef.current.focus();
    };
  }, [open, onClose]);
  if (!open) return null;
  const destructive = tone === 'destructive';
  return /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'fixed',
      inset: 0,
      zIndex: 60,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      padding: 16
    }
  }, /*#__PURE__*/React.createElement("div", {
    onClick: onClose,
    "aria-hidden": "true",
    style: {
      position: 'absolute',
      inset: 0,
      background: 'rgba(4,6,10,0.55)'
    }
  }), /*#__PURE__*/React.createElement("div", {
    ref: panelRef,
    role: "dialog",
    "aria-modal": "true",
    "aria-labelledby": titleId,
    style: {
      position: 'relative',
      width: 420,
      maxWidth: '100%',
      background: 'var(--surface-1)',
      border: '1px solid var(--border-default)',
      borderRadius: 'var(--radius-lg)',
      boxShadow: 'var(--shadow-overlay)',
      padding: 20
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'flex-start',
      justifyContent: 'space-between',
      gap: 12
    }
  }, /*#__PURE__*/React.createElement("span", {
    id: titleId,
    style: {
      fontSize: 'var(--fs-h4)',
      fontWeight: 'var(--fw-semibold)',
      letterSpacing: 'var(--ls-tight)',
      color: 'var(--text-primary)'
    }
  }, title), /*#__PURE__*/React.createElement(CloseBtn, {
    onClick: onClose
  })), description && /*#__PURE__*/React.createElement("p", {
    style: {
      fontSize: 'var(--fs-body-sm)',
      color: 'var(--text-secondary)',
      lineHeight: 'var(--lh-normal)',
      margin: '10px 0 0'
    }
  }, description), children && /*#__PURE__*/React.createElement("div", {
    style: {
      marginTop: 14
    }
  }, children), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      justifyContent: 'flex-end',
      gap: 8,
      marginTop: 20
    }
  }, onSecondary !== undefined && /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: onSecondary || onClose,
    style: {
      height: 34,
      padding: '0 14px',
      background: 'var(--surface-3)',
      color: 'var(--text-primary)',
      border: '1px solid var(--border-default)',
      borderRadius: 'var(--radius-md)',
      fontFamily: 'var(--font-sans)',
      fontSize: 'var(--fs-body)',
      fontWeight: 'var(--fw-medium)',
      cursor: 'pointer'
    }
  }, secondaryLabel), onPrimary && /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: onPrimary,
    style: {
      height: 34,
      padding: '0 14px',
      background: destructive ? 'var(--red-600)' : 'var(--accent)',
      color: '#fff',
      border: 'none',
      borderRadius: 'var(--radius-md)',
      fontFamily: 'var(--font-sans)',
      fontSize: 'var(--fs-body)',
      fontWeight: 'var(--fw-medium)',
      cursor: 'pointer'
    }
  }, primaryLabel))));
}
Object.assign(__ds_scope, { Modal });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/overlays/Modal.jsx", error: String((e && e.message) || e) }); }

// components/overlays/Toast.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const toneIcon = {
  success: {
    color: 'var(--status-success-fg)',
    d: 'M4 12.5 9 17.5 20 6.5'
  },
  warning: {
    color: 'var(--status-warning-fg)',
    d: 'M12 9v4M12 17v.01'
  },
  error: {
    color: 'var(--status-error-fg)',
    d: 'M6 6l12 12M18 6 6 18'
  },
  info: {
    color: 'var(--status-info-fg)',
    d: 'M12 8v.01M12 11v5'
  }
};

/**
 * Toast — one transient notification. Pauses its auto-dismiss timer on hover.
 * Render inside <ToastStack>, not standalone.
 */
function Toast({
  tone = 'info',
  title,
  description,
  onDismiss,
  duration = 4000
}) {
  const [paused, setPaused] = React.useState(false);
  const remaining = React.useRef(duration);
  const startedAt = React.useRef(Date.now());
  const timerRef = React.useRef(null);
  React.useEffect(() => {
    if (paused) {
      clearTimeout(timerRef.current);
      remaining.current -= Date.now() - startedAt.current;
      return;
    }
    startedAt.current = Date.now();
    timerRef.current = setTimeout(() => onDismiss && onDismiss(), remaining.current);
    return () => clearTimeout(timerRef.current);
  }, [paused]);
  const ic = toneIcon[tone] || toneIcon.info;
  return /*#__PURE__*/React.createElement("div", {
    role: "status",
    onMouseEnter: () => setPaused(true),
    onMouseLeave: () => setPaused(false),
    style: {
      display: 'flex',
      gap: 10,
      width: 340,
      maxWidth: '92vw',
      padding: 12,
      background: 'var(--surface-2)',
      border: '1px solid var(--border-default)',
      borderRadius: 'var(--radius-lg)',
      boxShadow: 'var(--shadow-overlay)',
      alignItems: 'flex-start'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      color: ic.color,
      flexShrink: 0,
      marginTop: 1
    }
  }, /*#__PURE__*/React.createElement("svg", {
    width: "16",
    height: "16",
    viewBox: "0 0 24 24",
    fill: "none",
    "aria-hidden": "true"
  }, /*#__PURE__*/React.createElement("path", {
    d: ic.d,
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round",
    strokeLinejoin: "round"
  }))), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      minWidth: 0
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-body-sm)',
      fontWeight: 'var(--fw-medium)',
      color: 'var(--text-primary)'
    }
  }, title), description && /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-caption)',
      color: 'var(--text-tertiary)',
      marginTop: 2,
      lineHeight: 'var(--lh-normal)'
    }
  }, description)), /*#__PURE__*/React.createElement("button", {
    type: "button",
    "aria-label": "Dismiss",
    onClick: onDismiss,
    style: {
      background: 'none',
      border: 'none',
      color: 'var(--text-tertiary)',
      cursor: 'pointer',
      flexShrink: 0,
      padding: 2
    }
  }, /*#__PURE__*/React.createElement("svg", {
    width: "13",
    height: "13",
    viewBox: "0 0 24 24",
    fill: "none",
    "aria-hidden": "true"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M6 6l12 12M18 6 6 18",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round"
  }))));
}

/**
 * ToastStack — fixed bottom-right stack. Shows at most `limit` toasts and a
 * "+N more" affordance when the queue is deeper than that.
 */
function ToastStack({
  toasts = [],
  onDismiss,
  limit = 4
}) {
  const visible = toasts.slice(0, limit);
  const overflow = toasts.length - visible.length;
  return /*#__PURE__*/React.createElement("div", {
    "aria-live": "polite",
    style: {
      position: 'fixed',
      right: 20,
      bottom: 20,
      zIndex: 70,
      display: 'flex',
      flexDirection: 'column-reverse',
      gap: 10
    }
  }, overflow > 0 && /*#__PURE__*/React.createElement("div", {
    style: {
      alignSelf: 'flex-end',
      fontSize: 'var(--fs-caption)',
      color: 'var(--text-tertiary)',
      fontFamily: 'var(--font-mono)'
    }
  }, "+", overflow, " more"), visible.map(t => /*#__PURE__*/React.createElement(Toast, _extends({
    key: t.id
  }, t, {
    onDismiss: () => onDismiss(t.id)
  }))));
}
Object.assign(__ds_scope, { Toast, ToastStack });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/overlays/Toast.jsx", error: String((e && e.message) || e) }); }

// components/overlays/Tooltip.jsx
try { (() => {
const sidePos = {
  top: {
    bottom: '100%',
    left: '50%',
    transform: 'translateX(-50%)',
    marginBottom: 6
  },
  bottom: {
    top: '100%',
    left: '50%',
    transform: 'translateX(-50%)',
    marginTop: 6
  },
  left: {
    right: '100%',
    top: '50%',
    transform: 'translateY(-50%)',
    marginRight: 6
  },
  right: {
    left: '100%',
    top: '50%',
    transform: 'translateY(-50%)',
    marginLeft: 6
  }
};

/**
 * Tooltip — for dense UI where labels truncate or icon-only controls need a
 * hint. Triggers on hover AND keyboard focus (not hover-only).
 */
function Tooltip({
  label,
  side = 'top',
  children
}) {
  const [open, setOpen] = React.useState(false);
  const id = React.useId();
  const show = () => setOpen(true);
  const hide = () => setOpen(false);
  const child = React.isValidElement(children) ? React.cloneElement(children, {
    'aria-describedby': id,
    onMouseEnter: e => {
      show();
      children.props.onMouseEnter && children.props.onMouseEnter(e);
    },
    onMouseLeave: e => {
      hide();
      children.props.onMouseLeave && children.props.onMouseLeave(e);
    },
    onFocus: e => {
      show();
      children.props.onFocus && children.props.onFocus(e);
    },
    onBlur: e => {
      hide();
      children.props.onBlur && children.props.onBlur(e);
    }
  }) : children;
  return /*#__PURE__*/React.createElement("span", {
    style: {
      position: 'relative',
      display: 'inline-flex'
    }
  }, child, open && /*#__PURE__*/React.createElement("span", {
    role: "tooltip",
    id: id,
    style: {
      position: 'absolute',
      ...sidePos[side],
      background: 'var(--surface-3)',
      border: '1px solid var(--border-default)',
      padding: '4px 8px',
      borderRadius: 'var(--radius-sm)',
      fontSize: 'var(--fs-caption)',
      color: 'var(--text-primary)',
      whiteSpace: 'nowrap',
      zIndex: 80,
      pointerEvents: 'none',
      boxShadow: 'var(--shadow-sm)'
    }
  }, label));
}
Object.assign(__ds_scope, { Tooltip });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/overlays/Tooltip.jsx", error: String((e && e.message) || e) }); }

// components/structure/Calendar.jsx
try { (() => {
const DAY_START = 7 * 60,
  DAY_END = 20 * 60,
  PX_PER_MIN = 1.15; // 07:00–20:00, ~56px/hour
const statusTone = {
  confirmed: {
    bg: 'var(--status-success-bg)',
    border: 'var(--status-success-border)',
    fg: 'var(--status-success-fg)'
  },
  rescheduled: {
    bg: 'var(--status-warning-bg)',
    border: 'var(--status-warning-border)',
    fg: 'var(--status-warning-fg)'
  },
  cancelled: {
    bg: 'var(--status-neutral-bg)',
    border: 'var(--status-neutral-border)',
    fg: 'var(--status-neutral-fg)'
  },
  'no-show': {
    bg: 'var(--status-error-bg)',
    border: 'var(--status-error-border)',
    fg: 'var(--status-error-fg)'
  }
};
function toMin(hhmm) {
  const [h, m] = hhmm.split(':').map(Number);
  return h * 60 + m;
}
function fmtHour(min) {
  const h = Math.floor(min / 60);
  const ampm = h < 12 ? 'AM' : 'PM';
  const h12 = h % 12 === 0 ? 12 : h % 12;
  return `${h12}${ampm}`;
}
function layoutColumn(appts) {
  const sorted = [...appts].sort((a, b) => toMin(a.start) - toMin(b.start));
  const groups = [];
  for (const a of sorted) {
    const g = groups.find(gr => gr.some(b => toMin(a.start) < toMin(b.end) && toMin(b.start) < toMin(a.end)));
    if (g) g.push(a);else groups.push([a]);
  }
  const placed = [];
  for (const g of groups) g.forEach((a, i) => placed.push({
    ...a,
    col: i,
    cols: g.length
  }));
  return placed;
}
function Slot({
  staffId,
  min,
  onSlotClick,
  working
}) {
  const [hover, setHover] = React.useState(false);
  return /*#__PURE__*/React.createElement("div", {
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    onClick: () => onSlotClick && working && onSlotClick(staffId, min),
    style: {
      position: 'absolute',
      left: 0,
      right: 0,
      top: (min - DAY_START) * PX_PER_MIN,
      height: 30 * PX_PER_MIN,
      cursor: working && onSlotClick ? 'pointer' : 'default',
      background: hover && working ? 'var(--accent-subtle)' : 'transparent',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      transition: 'var(--transition-colors)'
    }
  }, hover && working && onSlotClick && /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--text-accent)',
      fontSize: 'var(--fs-body)',
      fontWeight: 600
    }
  }, "+"));
}
function AppointmentBlock({
  a,
  onClick
}) {
  const tone = statusTone[a.status] || statusTone.confirmed;
  const top = (toMin(a.start) - DAY_START) * PX_PER_MIN;
  const height = Math.max(18, (toMin(a.end) - toMin(a.start)) * PX_PER_MIN - 2);
  const widthPct = 100 / a.cols,
    leftPct = widthPct * a.col;
  const dimmed = a.status === 'cancelled';
  return /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: () => onClick && onClick(a),
    style: {
      position: 'absolute',
      top,
      height,
      left: `calc(${leftPct}% + 2px)`,
      width: `calc(${widthPct}% - 4px)`,
      background: tone.bg,
      border: `1px solid ${tone.border}`,
      borderLeft: `2px solid ${tone.fg}`,
      borderRadius: 'var(--radius-sm)',
      padding: '4px 6px',
      textAlign: 'left',
      cursor: 'pointer',
      overflow: 'hidden',
      opacity: dimmed ? 0.6 : 1
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-caption)',
      fontWeight: 'var(--fw-medium)',
      color: 'var(--text-primary)',
      textDecoration: dimmed ? 'line-through' : 'none',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis'
    }
  }, a.title), /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-micro)',
      fontFamily: 'var(--font-mono)',
      color: tone.fg
    }
  }, a.start, "\\u2013", a.end));
}
function NowLine({
  min
}) {
  if (min < DAY_START || min > DAY_END) return null;
  return /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: 0,
      right: 0,
      top: (min - DAY_START) * PX_PER_MIN,
      height: 0,
      zIndex: 5,
      pointerEvents: 'none'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: -5,
      top: -4,
      width: 8,
      height: 8,
      borderRadius: '50%',
      background: 'var(--accent)'
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      borderTop: '1.5px solid var(--accent)'
    }
  }));
}

/**
 * CalendarGrid — one column per staff member, one row per time slot.
 * Shared rendering used by WeekCalendar and DayCalendar.
 */
function CalendarGrid({
  staff,
  workingHours = {
    start: '08:00',
    end: '18:00'
  },
  appointments = [],
  now,
  onSlotClick,
  onAppointmentClick
}) {
  const hours = [];
  for (let m = DAY_START; m <= DAY_END; m += 60) hours.push(m);
  const workStart = toMin(workingHours.start),
    workEnd = toMin(workingHours.end);
  const nowMin = now ? now.getHours() * 60 + now.getMinutes() : null;
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      border: '1px solid var(--border-default)',
      borderRadius: 'var(--radius-lg)',
      overflow: 'hidden',
      background: 'var(--surface-1)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      width: 56,
      flexShrink: 0,
      borderRight: '1px solid var(--border-subtle)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      height: 44,
      borderBottom: '1px solid var(--border-subtle)'
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'relative',
      height: (DAY_END - DAY_START) * PX_PER_MIN
    }
  }, hours.map(m => /*#__PURE__*/React.createElement("div", {
    key: m,
    style: {
      position: 'absolute',
      top: (m - DAY_START) * PX_PER_MIN - 6,
      right: 8,
      fontSize: 'var(--fs-micro)',
      fontFamily: 'var(--font-mono)',
      color: 'var(--text-tertiary)'
    }
  }, fmtHour(m))))), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      display: 'flex',
      overflowX: 'auto'
    }
  }, staff.map(s => {
    const col = layoutColumn(appointments.filter(a => a.staffId === s.id));
    return /*#__PURE__*/React.createElement("div", {
      key: s.id,
      style: {
        flex: '1 0 150px',
        minWidth: 150,
        borderRight: '1px solid var(--border-subtle)'
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        height: 44,
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '0 10px',
        borderBottom: '1px solid var(--border-subtle)',
        background: 'var(--surface-2)'
      }
    }, /*#__PURE__*/React.createElement("span", {
      style: {
        width: 22,
        height: 22,
        borderRadius: '50%',
        background: 'var(--surface-3)',
        border: '1px solid var(--border-default)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: 'var(--fs-micro)',
        color: 'var(--text-secondary)',
        flexShrink: 0
      }
    }, s.name.split(' ').map(p => p[0]).slice(0, 2).join('').toUpperCase()), /*#__PURE__*/React.createElement("span", {
      style: {
        fontSize: 'var(--fs-body-sm)',
        fontWeight: 'var(--fw-medium)',
        color: 'var(--text-primary)',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
      }
    }, s.name)), /*#__PURE__*/React.createElement("div", {
      style: {
        position: 'relative',
        height: (DAY_END - DAY_START) * PX_PER_MIN,
        backgroundImage: `repeating-linear-gradient(to bottom, transparent, transparent ${60 * PX_PER_MIN - 1}px, var(--border-subtle) ${60 * PX_PER_MIN}px)`
      }
    }, workStart > DAY_START && /*#__PURE__*/React.createElement("div", {
      style: {
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        height: (workStart - DAY_START) * PX_PER_MIN,
        background: 'repeating-linear-gradient(135deg, var(--surface-2) 0 6px, var(--surface-1) 6px 12px)'
      }
    }), workEnd < DAY_END && /*#__PURE__*/React.createElement("div", {
      style: {
        position: 'absolute',
        bottom: 0,
        left: 0,
        right: 0,
        height: (DAY_END - workEnd) * PX_PER_MIN,
        background: 'repeating-linear-gradient(135deg, var(--surface-2) 0 6px, var(--surface-1) 6px 12px)'
      }
    }), Array.from({
      length: Math.floor((DAY_END - DAY_START) / 30)
    }).map((_, i) => {
      const min = DAY_START + i * 30;
      const working = min >= workStart && min < workEnd;
      return /*#__PURE__*/React.createElement(Slot, {
        key: min,
        staffId: s.id,
        min: min,
        working: working,
        onSlotClick: onSlotClick
      });
    }), col.map((a, i) => /*#__PURE__*/React.createElement(AppointmentBlock, {
      key: a.id ?? i,
      a: a,
      onClick: onAppointmentClick
    })), nowMin != null && /*#__PURE__*/React.createElement(NowLine, {
      min: nowMin
    })));
  })));
}

/** WeekCalendar — day-chip week navigator + CalendarGrid for the selected day. */
function WeekCalendar({
  weekStart,
  staff,
  workingHours,
  appointments,
  now,
  selectedDay,
  onSelectDay,
  onSlotClick,
  onAppointmentClick
}) {
  const days = Array.from({
    length: 7
  }).map((_, i) => {
    const d = new Date(weekStart);
    d.setDate(d.getDate() + i);
    return d;
  });
  const isSame = (a, b) => a.toDateString() === b.toDateString();
  return /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      gap: 6,
      marginBottom: 12
    }
  }, days.map(d => {
    const active = isSame(d, selectedDay);
    const isToday = now && isSame(d, now);
    return /*#__PURE__*/React.createElement("button", {
      key: d.toISOString(),
      type: "button",
      onClick: () => onSelectDay && onSelectDay(d),
      style: {
        flex: 1,
        padding: '8px 4px',
        borderRadius: 'var(--radius-md)',
        border: `1px solid ${active ? 'var(--accent)' : 'var(--border-default)'}`,
        background: active ? 'var(--accent-subtle)' : 'var(--surface-1)',
        cursor: 'pointer',
        textAlign: 'center'
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        fontSize: 'var(--fs-micro)',
        letterSpacing: 'var(--ls-caps)',
        textTransform: 'uppercase',
        color: active ? 'var(--text-accent)' : 'var(--text-tertiary)'
      }
    }, d.toLocaleDateString(undefined, {
      weekday: 'short'
    })), /*#__PURE__*/React.createElement("div", {
      style: {
        fontSize: 'var(--fs-body)',
        fontWeight: 'var(--fw-semibold)',
        color: active ? 'var(--text-primary)' : 'var(--text-secondary)',
        marginTop: 2
      }
    }, d.getDate(), isToday && /*#__PURE__*/React.createElement("span", {
      style: {
        display: 'inline-block',
        width: 4,
        height: 4,
        borderRadius: '50%',
        background: 'var(--accent)',
        marginLeft: 4,
        verticalAlign: 'middle'
      }
    })));
  })), /*#__PURE__*/React.createElement(CalendarGrid, {
    staff: staff,
    workingHours: workingHours,
    appointments: appointments,
    now: now,
    onSlotClick: onSlotClick,
    onAppointmentClick: onAppointmentClick
  }));
}

/** DayCalendar — single-day grid with prev/next navigation. */
function DayCalendar({
  day,
  onPrevDay,
  onNextDay,
  staff,
  workingHours,
  appointments,
  now,
  onSlotClick,
  onAppointmentClick
}) {
  return /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 10,
      marginBottom: 12
    }
  }, /*#__PURE__*/React.createElement("button", {
    type: "button",
    "aria-label": "Previous day",
    onClick: onPrevDay,
    style: {
      width: 30,
      height: 30,
      borderRadius: 'var(--radius-md)',
      border: '1px solid var(--border-default)',
      background: 'var(--surface-2)',
      color: 'var(--text-secondary)',
      cursor: 'pointer'
    }
  }, "\u2039"), /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-h4)',
      fontWeight: 'var(--fw-semibold)',
      letterSpacing: 'var(--ls-tight)',
      color: 'var(--text-primary)'
    }
  }, day.toLocaleDateString(undefined, {
    weekday: 'long',
    month: 'short',
    day: 'numeric'
  })), /*#__PURE__*/React.createElement("button", {
    type: "button",
    "aria-label": "Next day",
    onClick: onNextDay,
    style: {
      width: 30,
      height: 30,
      borderRadius: 'var(--radius-md)',
      border: '1px solid var(--border-default)',
      background: 'var(--surface-2)',
      color: 'var(--text-secondary)',
      cursor: 'pointer'
    }
  }, "\u203A")), /*#__PURE__*/React.createElement(CalendarGrid, {
    staff: staff,
    workingHours: workingHours,
    appointments: appointments,
    now: now,
    onSlotClick: onSlotClick,
    onAppointmentClick: onAppointmentClick
  }));
}

/** MonthPicker — compact month grid for jumping between weeks/days. */
function MonthPicker({
  month,
  selected,
  today,
  onSelect
}) {
  const year = month.getFullYear(),
    m = month.getMonth();
  const first = new Date(year, m, 1);
  const startOffset = (first.getDay() + 6) % 7; // Monday-first
  const daysInMonth = new Date(year, m + 1, 0).getDate();
  const cells = Array.from({
    length: startOffset
  }).map(() => null).concat(Array.from({
    length: daysInMonth
  }, (_, i) => new Date(year, m, i + 1)));
  const isSame = (a, b) => a && b && a.toDateString() === b.toDateString();
  return /*#__PURE__*/React.createElement("div", {
    style: {
      width: 240,
      background: 'var(--surface-1)',
      border: '1px solid var(--border-default)',
      borderRadius: 'var(--radius-lg)',
      padding: 12
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 'var(--fs-body-sm)',
      fontWeight: 'var(--fw-semibold)',
      color: 'var(--text-primary)',
      marginBottom: 8,
      letterSpacing: 'var(--ls-normal)'
    }
  }, month.toLocaleDateString(undefined, {
    month: 'long',
    year: 'numeric'
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'grid',
      gridTemplateColumns: 'repeat(7,1fr)',
      gap: 2
    }
  }, ['M', 'T', 'W', 'T', 'F', 'S', 'S'].map((d, i) => /*#__PURE__*/React.createElement("div", {
    key: i,
    style: {
      textAlign: 'center',
      fontSize: 'var(--fs-micro)',
      color: 'var(--text-tertiary)',
      padding: '2px 0'
    }
  }, d)), cells.map((d, i) => /*#__PURE__*/React.createElement("button", {
    key: i,
    type: "button",
    disabled: !d,
    onClick: () => d && onSelect && onSelect(d),
    style: {
      aspectRatio: '1',
      border: 'none',
      borderRadius: 'var(--radius-sm)',
      cursor: d ? 'pointer' : 'default',
      background: isSame(d, selected) ? 'var(--accent)' : 'transparent',
      color: !d ? 'transparent' : isSame(d, selected) ? '#fff' : isSame(d, today) ? 'var(--text-accent)' : 'var(--text-secondary)',
      fontSize: 'var(--fs-caption)',
      fontWeight: isSame(d, today) ? 'var(--fw-semibold)' : 'var(--fw-regular)'
    }
  }, d ? d.getDate() : ''))));
}

/** Calendar — namespace export bundling the scheduler pieces (Calendar.Week, Calendar.Day, Calendar.MonthPicker). */
const Calendar = {
  Week: WeekCalendar,
  Day: DayCalendar,
  MonthPicker
};
Object.assign(__ds_scope, { WeekCalendar, DayCalendar, MonthPicker, Calendar });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/structure/Calendar.jsx", error: String((e && e.message) || e) }); }

// components/structure/Timeline.jsx
try { (() => {
const stateStyle = {
  pending: {
    color: 'var(--text-tertiary)',
    bg: 'var(--surface-1)'
  },
  running: {
    color: 'var(--text-accent)',
    bg: 'var(--surface-1)'
  },
  success: {
    color: 'var(--status-success-fg)',
    bg: 'var(--surface-1)'
  },
  retried: {
    color: 'var(--status-warning-fg)',
    bg: 'var(--surface-1)'
  },
  failed: {
    color: 'var(--status-error-fg)',
    bg: 'var(--surface-1)'
  }
};
function StepIcon({
  state
}) {
  const s = stateStyle[state] || stateStyle.pending;
  const common = {
    width: 11,
    height: 11,
    viewBox: '0 0 24 24',
    fill: 'none',
    'aria-hidden': true
  };
  let inner;
  if (state === 'success') inner = /*#__PURE__*/React.createElement("path", {
    d: "M4 12.5 9 17.5 20 6.5",
    stroke: s.color,
    strokeWidth: 3.4,
    strokeLinecap: "round",
    strokeLinejoin: "round"
  });else if (state === 'failed') inner = /*#__PURE__*/React.createElement("path", {
    d: "M6 6l12 12M18 6 6 18",
    stroke: s.color,
    strokeWidth: 3.2,
    strokeLinecap: "round"
  });else if (state === 'retried') inner = /*#__PURE__*/React.createElement("path", {
    d: "M20 11a8 8 0 1 0-.6 4M20 5v6h-6",
    stroke: s.color,
    strokeWidth: 3,
    strokeLinecap: "round",
    strokeLinejoin: "round"
  });else if (state === 'running') inner = /*#__PURE__*/React.createElement("circle", {
    cx: "12",
    cy: "12",
    r: "7",
    stroke: s.color,
    strokeWidth: 3,
    strokeDasharray: "30 12",
    style: {
      animation: 'kk-tl-spin 0.8s linear infinite',
      transformOrigin: '50% 50%'
    }
  });else inner = /*#__PURE__*/React.createElement("circle", {
    cx: "12",
    cy: "12",
    r: "4",
    fill: s.color
  });
  return /*#__PURE__*/React.createElement("svg", common, inner, /*#__PURE__*/React.createElement("style", null, '@keyframes kk-tl-spin{to{transform:rotate(360deg)}}'));
}
function Step({
  step,
  isLast,
  nested
}) {
  const [open, setOpen] = React.useState(false);
  const s = stateStyle[step.state] || stateStyle.pending;
  const dotSize = nested ? 16 : 20;
  return /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'relative',
      paddingBottom: isLast ? 0 : nested ? 14 : 18
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: -(nested ? 19 : 22),
      top: 2,
      width: dotSize,
      height: dotSize,
      borderRadius: '50%',
      background: 'var(--surface-1)',
      border: `2px solid ${s.color}`,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center'
    }
  }, /*#__PURE__*/React.createElement(StepIcon, {
    state: step.state
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 8,
      marginBottom: 3,
      flexWrap: 'wrap'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 'var(--fs-micro)',
      color: 'var(--text-tertiary)'
    }
  }, step.t), /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: nested ? 'var(--fs-caption)' : 'var(--fs-body-sm)',
      fontWeight: 'var(--fw-medium)',
      color: 'var(--text-primary)'
    }
  }, step.title), (step.duration || step.tokens != null) && /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 'var(--fs-micro)',
      color: 'var(--text-tertiary)'
    }
  }, step.duration, step.tokens != null ? ` · ${step.tokens}tok` : '')), step.detail && /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: step.mono ? 'var(--font-mono)' : 'var(--font-sans)',
      fontSize: 'var(--fs-caption)',
      color: 'var(--text-secondary)',
      lineHeight: 'var(--lh-normal)',
      background: step.mono ? 'var(--surface-inset)' : 'transparent',
      border: step.mono ? '1px solid var(--border-subtle)' : 'none',
      borderRadius: 6,
      padding: step.mono ? '6px 9px' : 0,
      marginBottom: step.payload ? 6 : 0
    }
  }, step.detail), step.payload && /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: () => setOpen(o => !o),
    style: {
      background: 'none',
      border: 'none',
      padding: 0,
      color: 'var(--text-accent)',
      fontSize: 'var(--fs-caption)',
      fontFamily: 'var(--font-sans)',
      fontWeight: 'var(--fw-medium)',
      cursor: 'pointer',
      marginBottom: open ? 6 : 0
    }
  }, open ? 'Hide payload' : 'Show payload'), open && /*#__PURE__*/React.createElement(__ds_scope.CodeBlock, {
    code: step.payload,
    title: step.payloadTitle || 'payload'
  })), step.retries && step.retries.length > 0 && /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'relative',
      marginTop: 10,
      paddingLeft: 19
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: 5,
      top: 2,
      bottom: 8,
      width: 1,
      background: 'var(--border-subtle)'
    }
  }), step.retries.map((r, i) => /*#__PURE__*/React.createElement(Step, {
    key: i,
    step: r,
    isLast: i === step.retries.length - 1,
    nested: true
  }))));
}

/**
 * Timeline — vertical agent-trace sequence. Each step: icon by state
 * (pending/running/success/retried/failed), monospace duration + token
 * count, an optional expandable payload (renders via CodeBlock), and nested
 * `retries` rendered as sub-attempts under their parent (never top-level).
 */
function Timeline({
  steps,
  loading,
  empty
}) {
  if (loading) return /*#__PURE__*/React.createElement(TimelineSkeleton, null);
  if (!steps || steps.length === 0) {
    return /*#__PURE__*/React.createElement(__ds_scope.EmptyState, {
      compact: true,
      title: empty?.title || 'No trace yet',
      description: empty?.description || 'This run has not produced any steps.'
    });
  }
  return /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'relative',
      paddingLeft: 22
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: 5,
      top: 4,
      bottom: 4,
      width: 1,
      background: 'var(--border-default)'
    }
  }), steps.map((s, i) => /*#__PURE__*/React.createElement(Step, {
    key: s.id ?? i,
    step: s,
    isLast: i === steps.length - 1
  })));
}

/** TimelineSkeleton — loading placeholder mirroring the step layout. */
function TimelineSkeleton({
  count = 4
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 18
    }
  }, Array.from({
    length: count
  }).map((_, i) => /*#__PURE__*/React.createElement("div", {
    key: i,
    style: {
      display: 'flex',
      gap: 10,
      alignItems: 'flex-start'
    }
  }, /*#__PURE__*/React.createElement(__ds_scope.Skeleton, {
    circle: true,
    width: 18
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      display: 'flex',
      flexDirection: 'column',
      gap: 6
    }
  }, /*#__PURE__*/React.createElement(__ds_scope.Skeleton, {
    width: "55%",
    height: 12
  }), /*#__PURE__*/React.createElement(__ds_scope.Skeleton, {
    width: "80%",
    height: 11
  })))));
}
Object.assign(__ds_scope, { Timeline, TimelineSkeleton });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/structure/Timeline.jsx", error: String((e && e.message) || e) }); }

// tailwind.config.js
try { (() => {
/** Kontor & DocMind — Tailwind mapping.
 *  Points class names at the live CSS custom properties in styles.css, so
 *  theme edits (a token value change) propagate without touching this file.
 *  Link styles.css in your app before Tailwind's base layer. */
/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./src/**/*.{html,js,jsx,ts,tsx}'],
  darkMode: ['selector', '[data-theme="dark"]'],
  theme: {
    extend: {
      colors: {
        'bg-base': 'var(--bg-base)',
        'surface-1': 'var(--surface-1)',
        'surface-2': 'var(--surface-2)',
        'surface-3': 'var(--surface-3)',
        'surface-hover': 'var(--surface-hover)',
        'surface-inset': 'var(--surface-inset)',
        'border-subtle': 'var(--border-subtle)',
        'border-default': 'var(--border-default)',
        'border-strong': 'var(--border-strong)',
        'border-focus': 'var(--border-focus)',
        'text-primary': 'var(--text-primary)',
        'text-secondary': 'var(--text-secondary)',
        'text-tertiary': 'var(--text-tertiary)',
        'text-disabled': 'var(--text-disabled)',
        'text-inverse': 'var(--text-inverse)',
        'text-accent': 'var(--text-accent)',
        'text-on-accent': 'var(--text-on-accent)',
        accent: {
          DEFAULT: 'var(--accent)',
          hover: 'var(--accent-hover)',
          active: 'var(--accent-active)',
          subtle: 'var(--accent-subtle)',
          border: 'var(--accent-border)'
        },
        status: {
          success: {
            fg: 'var(--status-success-fg)',
            bg: 'var(--status-success-bg)',
            border: 'var(--status-success-border)'
          },
          warning: {
            fg: 'var(--status-warning-fg)',
            bg: 'var(--status-warning-bg)',
            border: 'var(--status-warning-border)'
          },
          error: {
            fg: 'var(--status-error-fg)',
            bg: 'var(--status-error-bg)',
            border: 'var(--status-error-border)'
          },
          neutral: {
            fg: 'var(--status-neutral-fg)',
            bg: 'var(--status-neutral-bg)',
            border: 'var(--status-neutral-border)'
          },
          info: {
            fg: 'var(--status-info-fg)',
            bg: 'var(--status-info-bg)',
            border: 'var(--status-info-border)'
          }
        },
        viz: {
          1: 'var(--viz-1)',
          2: 'var(--viz-2)',
          3: 'var(--viz-3)',
          4: 'var(--viz-4)',
          5: 'var(--viz-5)',
          6: 'var(--viz-6)'
        },
        'skeleton-base': 'var(--skeleton-base)',
        'skeleton-shine': 'var(--skeleton-shine)'
      },
      fontFamily: {
        sans: ['var(--font-sans)'],
        mono: ['var(--font-mono)']
      },
      fontSize: {
        'display-2xl': ['var(--fs-display-2xl)', {
          lineHeight: 'var(--lh-tight)',
          letterSpacing: 'var(--ls-tighter)'
        }],
        'display-xl': ['var(--fs-display-xl)', {
          lineHeight: 'var(--lh-tight)',
          letterSpacing: 'var(--ls-tighter)'
        }],
        'display-lg': ['var(--fs-display-lg)', {
          lineHeight: 'var(--lh-tight)',
          letterSpacing: 'var(--ls-tighter)'
        }],
        h1: ['var(--fs-h1)', {
          lineHeight: 'var(--lh-snug)',
          letterSpacing: 'var(--ls-tight)'
        }],
        h2: ['var(--fs-h2)', {
          lineHeight: 'var(--lh-snug)',
          letterSpacing: 'var(--ls-tight)'
        }],
        h3: ['var(--fs-h3)', {
          lineHeight: 'var(--lh-snug)',
          letterSpacing: 'var(--ls-tight)'
        }],
        h4: ['var(--fs-h4)', {
          lineHeight: 'var(--lh-snug)',
          letterSpacing: 'var(--ls-normal)'
        }],
        'body-lg': ['var(--fs-body-lg)', {
          lineHeight: 'var(--lh-normal)',
          letterSpacing: 'var(--ls-normal)'
        }],
        body: ['var(--fs-body)', {
          lineHeight: 'var(--lh-normal)',
          letterSpacing: 'var(--ls-normal)'
        }],
        'body-sm': ['var(--fs-body-sm)', {
          lineHeight: 'var(--lh-normal)',
          letterSpacing: 'var(--ls-normal)'
        }],
        caption: ['var(--fs-caption)', {
          lineHeight: 'var(--lh-snug)',
          letterSpacing: 'var(--ls-normal)'
        }],
        micro: ['var(--fs-micro)', {
          lineHeight: 'var(--lh-snug)',
          letterSpacing: 'var(--ls-caps)'
        }]
      },
      spacing: {
        px: 'var(--space-px)',
        '0.5': 'var(--space-0-5)',
        '1': 'var(--space-1)',
        '1.5': 'var(--space-1-5)',
        '2': 'var(--space-2)',
        '2.5': 'var(--space-2-5)',
        '3': 'var(--space-3)',
        '4': 'var(--space-4)',
        '5': 'var(--space-5)',
        '6': 'var(--space-6)',
        '8': 'var(--space-8)',
        '10': 'var(--space-10)',
        '12': 'var(--space-12)',
        '16': 'var(--space-16)',
        '20': 'var(--space-20)',
        '24': 'var(--space-24)',
        '32': 'var(--space-32)',
        'sidebar': 'var(--sidebar-width)',
        'sidebar-collapsed': 'var(--sidebar-width-collapsed)',
        'topbar': 'var(--topbar-height)'
      },
      borderRadius: {
        none: 'var(--radius-none)',
        xs: 'var(--radius-xs)',
        sm: 'var(--radius-sm)',
        DEFAULT: 'var(--radius-md)',
        md: 'var(--radius-md)',
        lg: 'var(--radius-lg)',
        xl: 'var(--radius-xl)',
        '2xl': 'var(--radius-2xl)',
        full: 'var(--radius-full)'
      },
      boxShadow: {
        none: 'var(--shadow-none)',
        sm: 'var(--shadow-sm)',
        overlay: 'var(--shadow-overlay)',
        focus: 'var(--shadow-focus)'
      },
      transitionDuration: {
        instant: 'var(--dur-instant)',
        fast: 'var(--dur-fast)',
        DEFAULT: 'var(--dur-base)',
        base: 'var(--dur-base)',
        slow: 'var(--dur-slow)'
      },
      transitionTimingFunction: {
        out: 'var(--ease-out)',
        'in-out': 'var(--ease-in-out)',
        standard: 'var(--ease-standard)'
      }
    }
  },
  plugins: []
};
})(); } catch (e) { __ds_ns.__errors.push({ path: "tailwind.config.js", error: String((e && e.message) || e) }); }

// ui_kits/chat-widget/ChatWidget.jsx
try { (() => {
/* Embeddable RAG chat widget — Kontor & DocMind "DocMind" assistant.
   Answers strictly from company documents, with inline source citations.
   Self-contained (uses DS tokens via inline styles + a few DS primitives). */
(function () {
  const React = window.React;
  const {
    useState,
    useRef,
    useEffect
  } = React;
  const DS = () => window.KontorKanonDesignSystem_452420;
  const I = () => window.KKIcons;
  const seed = [{
    role: 'assistant',
    text: "Hi — I'm DocMind, Acme Plumbing's assistant. I answer from our published docs. What can I help with?",
    sources: []
  }];
  const canned = {
    text: "Emergency call-outs are available 24/7. Outside 08:00–18:00 on weekdays a €45 out-of-hours surcharge applies, waived for existing service-plan members.",
    sources: [{
      id: 'doc_pricing',
      title: 'Pricing & call-out fees',
      page: 'p.3'
    }, {
      id: 'doc_hours',
      title: 'Service hours policy',
      page: '§2'
    }]
  };
  function Bubble({
    m
  }) {
    const isUser = m.role === 'user';
    const ic = I();
    return React.createElement('div', {
      style: {
        display: 'flex',
        flexDirection: 'column',
        alignItems: isUser ? 'flex-end' : 'flex-start',
        gap: 6
      }
    }, React.createElement('div', {
      style: {
        maxWidth: '84%',
        padding: '9px 12px',
        borderRadius: isUser ? '12px 12px 3px 12px' : '12px 12px 12px 3px',
        background: isUser ? 'var(--accent)' : 'var(--surface-3)',
        color: isUser ? 'var(--text-on-accent)' : 'var(--text-primary)',
        border: isUser ? 'none' : '1px solid var(--border-subtle)',
        fontSize: 'var(--fs-body-sm)',
        lineHeight: 'var(--lh-normal)'
      }
    }, m.text), m.sources && m.sources.length > 0 && React.createElement('div', {
      style: {
        display: 'flex',
        flexWrap: 'wrap',
        gap: 6,
        maxWidth: '90%'
      }
    }, m.sources.map(s => React.createElement('div', {
      key: s.id,
      style: {
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        padding: '3px 8px',
        background: 'var(--surface-inset)',
        border: '1px solid var(--border-subtle)',
        borderRadius: 'var(--radius-sm)',
        fontSize: 'var(--fs-micro)',
        color: 'var(--text-secondary)'
      }
    }, React.createElement('span', {
      style: {
        color: 'var(--text-accent)',
        display: 'flex'
      }
    }, ic.FileText({
      size: 12
    })), s.title, React.createElement('span', {
      style: {
        fontFamily: 'var(--font-mono)',
        color: 'var(--text-tertiary)'
      }
    }, s.page)))), !isUser && m.sources && React.createElement('div', {
      style: {
        display: 'flex',
        gap: 4,
        marginTop: 1
      }
    }, React.createElement('span', {
      style: {
        color: 'var(--text-tertiary)',
        display: 'flex',
        cursor: 'pointer'
      }
    }, ic.ThumbUp({
      size: 13
    })), React.createElement('span', {
      style: {
        color: 'var(--text-tertiary)',
        display: 'flex',
        cursor: 'pointer'
      }
    }, ic.ThumbDown({
      size: 13
    }))));
  }
  function TypingDots() {
    return React.createElement('div', {
      style: {
        display: 'flex',
        gap: 4,
        padding: '11px 13px',
        background: 'var(--surface-3)',
        border: '1px solid var(--border-subtle)',
        borderRadius: '12px 12px 12px 3px',
        width: 'fit-content'
      }
    }, [0, 1, 2].map(i => React.createElement('span', {
      key: i,
      style: {
        width: 6,
        height: 6,
        borderRadius: '50%',
        background: 'var(--text-tertiary)',
        animation: 'kkbounce 1s ease-in-out ' + i * 0.15 + 's infinite'
      }
    })), React.createElement('style', null, '@keyframes kkbounce{0%,80%,100%{opacity:.3;transform:translateY(0)}40%{opacity:1;transform:translateY(-3px)}}'));
  }
  function ChatPanel({
    onClose
  }) {
    const ds = DS();
    const ic = I();
    const [msgs, setMsgs] = useState(seed);
    const [val, setVal] = useState('');
    const [typing, setTyping] = useState(false);
    const scroller = useRef(null);
    useEffect(() => {
      if (scroller.current) scroller.current.scrollTop = scroller.current.scrollHeight;
    }, [msgs, typing]);
    const send = text => {
      const t = (text || val).trim();
      if (!t) return;
      setMsgs(m => [...m, {
        role: 'user',
        text: t
      }]);
      setVal('');
      setTyping(true);
      setTimeout(() => {
        setTyping(false);
        setMsgs(m => [...m, {
          role: 'assistant',
          text: canned.text,
          sources: canned.sources
        }]);
      }, 1100);
    };
    return React.createElement('div', {
      style: {
        width: 380,
        height: 560,
        display: 'flex',
        flexDirection: 'column',
        background: 'var(--surface-1)',
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius-xl)',
        overflow: 'hidden',
        boxShadow: 'var(--shadow-overlay)'
      }
    },
    // header
    React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        padding: '13px 14px',
        borderBottom: '1px solid var(--border-subtle)',
        flexShrink: 0
      }
    }, React.createElement('div', {
      style: {
        width: 32,
        height: 32,
        borderRadius: 9,
        background: 'var(--accent-subtle)',
        border: '1px solid var(--accent-border)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: 'var(--text-accent)'
      }
    }, ic.Bot({
      size: 18
    })), React.createElement('div', {
      style: {
        flex: 1
      }
    }, React.createElement('div', {
      style: {
        fontSize: 'var(--fs-body-sm)',
        fontWeight: 600,
        letterSpacing: 'var(--ls-normal)'
      }
    }, 'DocMind'), React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 5,
        fontSize: 'var(--fs-micro)',
        color: 'var(--text-tertiary)'
      }
    }, React.createElement('span', {
      style: {
        width: 6,
        height: 6,
        borderRadius: '50%',
        background: 'var(--status-success-fg)'
      }
    }), 'Answers from your docs')), React.createElement(ds.IconButton, {
      label: 'Close',
      icon: ic.X({
        size: 16
      }),
      onClick: onClose
    })),
    // messages
    React.createElement('div', {
      ref: scroller,
      role: 'log',
      'aria-live': 'polite',
      'aria-relevant': 'additions',
      style: {
        flex: 1,
        overflowY: 'auto',
        padding: 14,
        display: 'flex',
        flexDirection: 'column',
        gap: 14
      }
    }, msgs.map((m, i) => React.createElement(Bubble, {
      key: i,
      m
    })), typing && React.createElement(TypingDots)),
    // suggestions + input
    msgs.length <= 1 && React.createElement('div', {
      style: {
        display: 'flex',
        gap: 7,
        padding: '0 14px 10px',
        flexWrap: 'wrap'
      }
    }, ['Emergency call-out fees?', 'Do you serve my area?', 'Book a visit'].map(s => React.createElement('button', {
      key: s,
      onClick: () => send(s),
      style: {
        padding: '5px 10px',
        background: 'var(--surface-2)',
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius-full)',
        color: 'var(--text-secondary)',
        fontSize: 'var(--fs-caption)',
        fontFamily: 'var(--font-sans)',
        cursor: 'pointer'
      }
    }, s))), React.createElement('div', {
      style: {
        padding: 12,
        borderTop: '1px solid var(--border-subtle)',
        flexShrink: 0
      }
    }, React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '4px 4px 4px 12px',
        background: 'var(--surface-inset)',
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius-full)'
      }
    }, React.createElement('input', {
      value: val,
      onChange: e => setVal(e.target.value),
      onKeyDown: e => {
        if (e.key === 'Enter') send();
      },
      placeholder: 'Ask about services, hours, pricing…',
      'aria-label': 'Message DocMind',
      style: {
        flex: 1,
        border: 'none',
        outline: 'none',
        background: 'transparent',
        color: 'var(--text-primary)',
        fontFamily: 'var(--font-sans)',
        fontSize: 'var(--fs-body-sm)'
      }
    }), React.createElement(ds.IconButton, {
      label: 'Send',
      variant: 'primary',
      size: 'sm',
      icon: ic.Send({
        size: 15
      }),
      onClick: () => send()
    })), React.createElement('div', {
      style: {
        textAlign: 'center',
        fontSize: 'var(--fs-micro)',
        color: 'var(--text-tertiary)',
        marginTop: 8
      }
    }, 'Grounded in Acme Plumbing docs · Powered by Kontor & DocMind')));
  }
  function ChatWidget() {
    const ic = I();
    const [open, setOpen] = useState(true);
    return React.createElement('div', {
      'data-theme': 'dark',
      style: {
        position: 'relative',
        width: '100%',
        height: '100vh',
        background: 'var(--bg-base)',
        backgroundImage: 'radial-gradient(circle at 20% 20%, rgba(110,120,240,0.08), transparent 45%), radial-gradient(circle at 80% 70%, rgba(86,183,201,0.06), transparent 40%)',
        fontFamily: 'var(--font-sans)',
        overflow: 'hidden'
      }
    },
    // faux host site content
    React.createElement('div', {
      style: {
        padding: '48px 56px',
        maxWidth: 620
      }
    }, React.createElement('div', {
      style: {
        fontSize: 'var(--fs-caption)',
        letterSpacing: 'var(--ls-caps)',
        textTransform: 'uppercase',
        color: 'var(--text-tertiary)',
        fontWeight: 500
      }
    }, 'Acme Plumbing'), React.createElement('h1', {
      style: {
        fontSize: 'var(--fs-display-lg)',
        fontWeight: 600,
        letterSpacing: 'var(--ls-tighter)',
        color: 'var(--text-primary)',
        margin: '14px 0 12px',
        lineHeight: 'var(--lh-tight)'
      }
    }, 'Fast, honest plumbing — booked in seconds.'), React.createElement('p', {
      style: {
        fontSize: 'var(--fs-body-lg)',
        color: 'var(--text-secondary)',
        lineHeight: 'var(--lh-relaxed)',
        margin: 0
      }
    }, 'The widget in the corner answers customer questions from your real documents and books jobs straight into your calendar.')),
    // widget dock
    React.createElement('div', {
      style: {
        position: 'absolute',
        right: 24,
        bottom: 24,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'flex-end',
        gap: 14
      }
    }, open && React.createElement(ChatPanel, {
      onClose: () => setOpen(false)
    }), React.createElement('button', {
      onClick: () => setOpen(o => !o),
      'aria-label': 'Open chat',
      style: {
        width: 54,
        height: 54,
        borderRadius: '50%',
        background: 'var(--accent)',
        color: '#fff',
        border: 'none',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        cursor: 'pointer',
        boxShadow: 'var(--shadow-overlay)',
        transition: 'transform var(--dur-base) var(--ease-out)'
      }
    }, open ? ic.ChevronDown({
      size: 24
    }) : ic.Message({
      size: 24
    }))));
  }
  window.KKKit = window.KKKit || {};
  window.KKKit.ChatWidget = ChatWidget;
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/chat-widget/ChatWidget.jsx", error: String((e && e.message) || e) }); }

// ui_kits/marketing-landing/LandingPage.jsx
try { (() => {
/* Marketing landing page — Kontor & DocMind. Same tokens/components as the app,
   translated to a light-touch marketing layout (subtle glow, no gradients-in-product rule applies to app only). */
(function () {
  const React = window.React;
  const {
    useState
  } = React;
  const DS = () => window.KontorKanonDesignSystem_452420;
  const I = () => window.KKIcons;
  function Nav() {
    const ic = I();
    return React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        maxWidth: 1120,
        margin: '0 auto',
        padding: '20px 24px'
      }
    }, React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 9
      }
    }, React.createElement('div', {
      style: {
        width: 26,
        height: 26,
        borderRadius: 7,
        background: 'var(--accent)',
        color: '#fff',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontWeight: 600,
        fontSize: 'var(--fs-caption)',
        letterSpacing: 'var(--ls-tighter)'
      }
    }, 'K&K'), React.createElement('span', {
      style: {
        fontWeight: 600,
        fontSize: 'var(--fs-body)',
        letterSpacing: 'var(--ls-tight)',
        color: 'var(--text-primary)'
      }
    }, 'Kontor & DocMind')), React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 28
      }
    }, ['Product', 'Pricing', 'Docs'].map(l => React.createElement('a', {
      key: l,
      href: '#',
      style: {
        fontSize: 'var(--fs-body)',
        color: 'var(--text-secondary)'
      }
    }, l)), React.createElement(DS().Button, {
      variant: 'secondary',
      size: 'sm'
    }, 'Sign in'), React.createElement(DS().Button, {
      size: 'sm',
      trailingIcon: ic.ArrowRight({
        size: 14
      })
    }, 'Get started')));
  }
  function Hero() {
    const ic = I();
    return React.createElement('div', {
      style: {
        position: 'relative',
        overflow: 'hidden'
      }
    }, React.createElement('div', {
      style: {
        position: 'absolute',
        inset: 0,
        backgroundImage: 'radial-gradient(circle at 25% 20%, rgba(110,120,240,0.12), transparent 45%), radial-gradient(circle at 80% 0%, rgba(86,183,201,0.08), transparent 40%)',
        pointerEvents: 'none'
      }
    }), React.createElement('div', {
      style: {
        position: 'relative',
        maxWidth: 780,
        margin: '0 auto',
        padding: '88px 24px 64px',
        textAlign: 'center'
      }
    }, React.createElement(DS().Badge, {
      tone: 'info',
      style: {
        marginBottom: 22
      }
    }, 'Self-hosted · your infrastructure'), React.createElement('h1', {
      style: {
        fontSize: 'var(--fs-display-xl)',
        fontWeight: 600,
        letterSpacing: 'var(--ls-tighter)',
        lineHeight: 'var(--lh-tight)',
        color: 'var(--text-primary)',
        margin: '0 0 20px'
      }
    }, 'AI that books jobs and answers questions — grounded in your business.'), React.createElement('p', {
      style: {
        fontSize: 'var(--fs-h4)',
        color: 'var(--text-secondary)',
        lineHeight: 'var(--lh-relaxed)',
        maxWidth: 560,
        margin: '0 auto 32px'
      }
    }, 'Kontor acts in your calendar and CRM. DocMind answers strictly from your own documents. Both run on infrastructure you control.'), React.createElement('div', {
      style: {
        display: 'flex',
        gap: 12,
        justifyContent: 'center'
      }
    }, React.createElement(DS().Button, {
      size: 'lg',
      trailingIcon: ic.ArrowRight({
        size: 16
      })
    }, 'Start free trial'), React.createElement(DS().Button, {
      size: 'lg',
      variant: 'secondary',
      leadingIcon: ic.Play({
        size: 16
      })
    }, 'Watch demo'))));
  }
  function ProductSplit() {
    const ds = DS();
    const ic = I();
    const items = [{
      icon: ic.Bot,
      tag: 'Kontor',
      tone: 'info',
      title: 'Books appointments. Acts in your CRM.',
      body: 'Reads calendars, checks availability, confirms bookings, and updates the CRM record — all inside one traceable agent run.',
      bullets: ['Native calendar + CRM tool-calling', 'Confidence-gated auto-confirm', 'Full agent trace on every run']
    }, {
      icon: ic.Database,
      tag: 'DocMind',
      tone: 'success',
      title: 'Answers strictly from your documents.',
      body: 'Retrieval-augmented answers cite the source document and page — never invents policy, pricing, or hours.',
      bullets: ['Inline source citations', 'Refuses to answer off-corpus', 'Embeds as a widget in minutes']
    }];
    return React.createElement('div', {
      style: {
        maxWidth: 1120,
        margin: '0 auto',
        padding: '20px 24px 80px',
        display: 'grid',
        gridTemplateColumns: '1fr 1fr',
        gap: 20
      }
    }, items.map(it => React.createElement(ds.Card, {
      key: it.tag,
      padding: 'lg'
    }, React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        marginBottom: 18
      }
    }, React.createElement('div', {
      style: {
        width: 38,
        height: 38,
        borderRadius: 10,
        background: 'var(--surface-3)',
        border: '1px solid var(--border-default)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: 'var(--text-accent)'
      }
    }, it.icon({
      size: 19
    })), React.createElement(ds.Badge, {
      tone: it.tone
    }, it.tag)), React.createElement('h3', {
      style: {
        fontSize: 'var(--fs-h2)',
        fontWeight: 600,
        letterSpacing: 'var(--ls-tight)',
        color: 'var(--text-primary)',
        margin: '0 0 10px'
      }
    }, it.title), React.createElement('p', {
      style: {
        fontSize: 'var(--fs-body)',
        color: 'var(--text-secondary)',
        lineHeight: 'var(--lh-relaxed)',
        margin: '0 0 18px'
      }
    }, it.body), React.createElement('div', {
      style: {
        display: 'flex',
        flexDirection: 'column',
        gap: 9
      }
    }, it.bullets.map(b => React.createElement('div', {
      key: b,
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 9,
        fontSize: 'var(--fs-body-sm)',
        color: 'var(--text-secondary)'
      }
    }, React.createElement('span', {
      style: {
        color: 'var(--status-success-fg)',
        display: 'flex'
      }
    }, ic.Check({
      size: 15
    })), b))))));
  }
  function SurfaceStrip() {
    const ic = I();
    const rows = [{
      icon: ic.Message,
      title: 'Chat widget',
      body: 'Drop-in embed for your site — cited answers, escalation to a human.'
    }, {
      icon: ic.BarChart,
      title: 'Operator dashboard',
      body: 'Runs, agent trace timeline, and analytics in one dense workspace.'
    }, {
      icon: ic.Globe,
      title: 'Marketing site',
      body: 'The same design language, tuned for a first-time visitor.'
    }];
    return React.createElement('div', {
      style: {
        borderTop: '1px solid var(--border-subtle)',
        borderBottom: '1px solid var(--border-subtle)',
        background: 'var(--surface-1)'
      }
    }, React.createElement('div', {
      style: {
        maxWidth: 1120,
        margin: '0 auto',
        padding: '48px 24px',
        display: 'grid',
        gridTemplateColumns: 'repeat(3,1fr)',
        gap: 32
      }
    }, rows.map(r => React.createElement('div', {
      key: r.title
    }, React.createElement('div', {
      style: {
        color: 'var(--text-accent)',
        marginBottom: 12
      }
    }, r.icon({
      size: 20
    })), React.createElement('div', {
      style: {
        fontSize: 'var(--fs-body)',
        fontWeight: 600,
        color: 'var(--text-primary)',
        marginBottom: 6,
        letterSpacing: 'var(--ls-normal)'
      }
    }, r.title), React.createElement('div', {
      style: {
        fontSize: 'var(--fs-body-sm)',
        color: 'var(--text-tertiary)',
        lineHeight: 'var(--lh-normal)'
      }
    }, r.body)))));
  }
  function Stats() {
    const stats = [['84%', 'deflection rate'], ['1.3s', 'median agent latency'], ['0', 'off-corpus answers'], ['100%', 'self-hosted']];
    return React.createElement('div', {
      style: {
        maxWidth: 1120,
        margin: '0 auto',
        padding: '56px 24px',
        display: 'flex',
        gap: 8
      }
    }, stats.map(([v, l]) => React.createElement('div', {
      key: l,
      style: {
        flex: 1,
        textAlign: 'center'
      }
    }, React.createElement('div', {
      style: {
        fontFamily: 'var(--font-mono)',
        fontSize: 'var(--fs-h1)',
        fontWeight: 500,
        color: 'var(--text-primary)',
        letterSpacing: 'var(--ls-tight)'
      }
    }, v), React.createElement('div', {
      style: {
        fontSize: 'var(--fs-body-sm)',
        color: 'var(--text-tertiary)',
        marginTop: 4
      }
    }, l))));
  }
  function CTA() {
    const ic = I();
    return React.createElement('div', {
      style: {
        maxWidth: 1120,
        margin: '0 auto',
        padding: '0 24px 96px'
      }
    }, React.createElement(DS().Card, {
      padding: 'lg',
      style: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        background: 'var(--surface-2)'
      }
    }, React.createElement('div', null, React.createElement('div', {
      style: {
        fontSize: 'var(--fs-h2)',
        fontWeight: 600,
        letterSpacing: 'var(--ls-tight)',
        color: 'var(--text-primary)'
      }
    }, 'Run it on your own servers.'), React.createElement('div', {
      style: {
        fontSize: 'var(--fs-body)',
        color: 'var(--text-tertiary)',
        marginTop: 6
      }
    }, 'No data leaves your infrastructure. Deploy in an afternoon.')), React.createElement(DS().Button, {
      size: 'lg',
      trailingIcon: ic.ArrowRight({
        size: 16
      })
    }, 'Talk to engineering')));
  }
  function LandingPage() {
    return React.createElement('div', {
      'data-theme': 'dark',
      style: {
        background: 'var(--bg-base)',
        minHeight: '100vh',
        fontFamily: 'var(--font-sans)'
      }
    }, React.createElement(Nav), React.createElement(Hero), React.createElement(ProductSplit), React.createElement(SurfaceStrip), React.createElement(Stats), React.createElement(CTA));
  }
  window.KKKit = window.KKKit || {};
  window.KKKit.LandingPage = LandingPage;
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/marketing-landing/LandingPage.jsx", error: String((e && e.message) || e) }); }

// ui_kits/operator-dashboard/OperatorDashboard.jsx
try { (() => {
/* Operator Dashboard — agent product surface for Kontor & DocMind.
   Composes DS primitives (Sidebar, DataTable, Card, Badge, Tabs, Button…).
   Reads the DS namespace + icons lazily at render (never at module top level). */
(function () {
  const React = window.React;
  const {
    useState
  } = React;
  const DS = () => window.KontorKanonDesignSystem_452420;
  const I = () => window.KKIcons;
  const runs = [{
    id: 'run_7f3a9c',
    intent: 'Book appointment',
    channel: 'WhatsApp',
    status: 'ok',
    conf: 0.96,
    latency: '1.24s',
    when: '14:32'
  }, {
    id: 'run_2b81de',
    intent: 'Reschedule visit',
    channel: 'Web chat',
    status: 'ok',
    conf: 0.91,
    latency: '0.98s',
    when: '14:19'
  }, {
    id: 'run_e04c17',
    intent: 'Answer from docs',
    channel: 'Widget',
    status: 'review',
    conf: 0.62,
    latency: '2.11s',
    when: '13:58'
  }, {
    id: 'run_9a55f0',
    intent: 'Cancel booking',
    channel: 'SMS',
    status: 'error',
    conf: 0.34,
    latency: '—',
    when: '13:40'
  }, {
    id: 'run_c17b22',
    intent: 'Book appointment',
    channel: 'WhatsApp',
    status: 'ok',
    conf: 0.98,
    latency: '1.02s',
    when: '13:21'
  }, {
    id: 'run_44de9a',
    intent: 'Pricing question',
    channel: 'Widget',
    status: 'ok',
    conf: 0.88,
    latency: '1.47s',
    when: '12:55'
  }, {
    id: 'run_ab7701',
    intent: 'Reschedule visit',
    channel: 'Web chat',
    status: 'review',
    conf: 0.58,
    latency: '2.44s',
    when: '12:30'
  }];
  const trace = [{
    t: '+0ms',
    title: 'User message received',
    state: 'success',
    detail: '"Can I move my Tuesday appointment to Thursday?"'
  }, {
    t: '+40ms',
    title: 'Intent classified → reschedule',
    state: 'success',
    detail: 'confidence 0.91 · model gpt-4o'
  }, {
    t: '+220ms',
    title: 'CRM.lookup_customer',
    state: 'retried',
    mono: true,
    detail: 'timeout after 2s',
    payload: {
      phone: '+31 6 •••• 4821'
    },
    retries: [{
      t: '+2.1s',
      title: 'CRM.lookup_customer (retry 1)',
      state: 'success',
      mono: true,
      detail: '1 match',
      duration: '180ms'
    }]
  }, {
    t: '+2.4s',
    title: 'Calendar.find_slots',
    state: 'success',
    mono: true,
    detail: 'Thu 07-24 → [09:00, 11:30, 15:00]'
  }, {
    t: '+2.7s',
    title: 'Calendar.book_slot',
    state: 'success',
    mono: true,
    duration: '210ms',
    tokens: 180,
    payload: {
      slot: '2026-07-24T11:30',
      status: 'confirmed'
    }
  }, {
    t: '+2.9s',
    title: 'Reply sent',
    state: 'success',
    detail: '"Done — moved to Thursday 24 July, 11:30. See you then."'
  }];
  const toneMap = {
    ok: ['success', 'Booked'],
    review: ['warning', 'Needs review'],
    error: ['error', 'Failed']
  };
  function Wordmark() {
    return React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 9
      }
    }, React.createElement('div', {
      style: {
        width: 26,
        height: 26,
        borderRadius: 7,
        background: 'var(--accent)',
        color: '#fff',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontWeight: 600,
        fontSize: 'var(--fs-caption)',
        letterSpacing: 'var(--ls-tighter)'
      }
    }, 'K&K'), React.createElement('span', {
      style: {
        fontWeight: 600,
        fontSize: 'var(--fs-body)',
        letterSpacing: 'var(--ls-tight)',
        color: 'var(--text-primary)'
      }
    }, 'Kontor ', React.createElement('span', {
      style: {
        color: 'var(--text-tertiary)',
        fontWeight: 400
      }
    }, '&'), ' DocMind'));
  }
  function Kpi({
    label,
    value,
    delta,
    tone
  }) {
    return React.createElement(DS().Card, {
      padding: 'md',
      style: {
        flex: 1
      }
    }, React.createElement('div', {
      style: {
        fontSize: 'var(--fs-micro)',
        fontWeight: 500,
        letterSpacing: 'var(--ls-caps)',
        textTransform: 'uppercase',
        color: 'var(--text-tertiary)'
      }
    }, label), React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'baseline',
        gap: 10,
        marginTop: 8
      }
    }, React.createElement('span', {
      style: {
        fontSize: 'var(--fs-h1)',
        fontWeight: 600,
        letterSpacing: 'var(--ls-tight)',
        color: 'var(--text-primary)'
      }
    }, value), delta && React.createElement('span', {
      style: {
        fontFamily: 'var(--font-mono)',
        fontSize: 'var(--fs-caption)',
        color: tone === 'down' ? 'var(--status-error-fg)' : 'var(--status-success-fg)'
      }
    }, delta)));
  }
  function TracePanel({
    run,
    open,
    onClose
  }) {
    const ds = DS();
    const ic = I();
    return React.createElement(ds.Drawer, {
      open,
      onClose,
      title: React.createElement('span', {
        style: {
          display: 'flex',
          alignItems: 'center',
          gap: 8
        }
      }, React.createElement('span', {
        style: {
          color: 'var(--text-accent)',
          display: 'flex'
        }
      }, ic.Activity({
        size: 16
      })), 'Agent trace')
    }, run && React.createElement(React.Fragment, null, React.createElement('div', {
      style: {
        marginBottom: 14
      }
    }, React.createElement(ds.KeyValueList, {
      items: [{
        label: 'Run',
        value: run.id,
        mono: true
      }, {
        label: 'Channel',
        value: run.channel
      }, {
        label: 'Status',
        value: React.createElement(ds.Badge, {
          tone: toneMap[run.status][0],
          dot: true
        }, toneMap[run.status][1])
      }, {
        label: 'Latency',
        value: run.latency,
        mono: true
      }, {
        label: 'Confidence',
        value: run.conf,
        mono: true
      }]
    })), React.createElement(ds.Timeline, {
      steps: trace
    })));
  }
  function OperatorDashboard() {
    const ds = DS();
    const ic = I();
    const [tab, setTab] = useState('runs');
    const [openRun, setOpenRun] = useState(null);
    const [theme, setTheme] = useState('dark');
    const lastTriggerRef = React.useRef(null);
    const openTrace = row => {
      lastTriggerRef.current = document.activeElement;
      setOpenRun(row);
    };
    const closeTrace = () => {
      setOpenRun(null);
      if (lastTriggerRef.current && lastTriggerRef.current.focus) lastTriggerRef.current.focus();
    };
    const nav = (Item, Section) => React.createElement(React.Fragment, null, React.createElement(Section, {
      label: 'Overview'
    }, React.createElement(Item, {
      icon: ic.Home({
        size: 16
      }),
      active: true
    }, 'Dashboard'), React.createElement(Item, {
      icon: ic.Inbox({
        size: 16
      }),
      badge: '12'
    }, 'Inbox'), React.createElement(Item, {
      icon: ic.Calendar({
        size: 16
      })
    }, 'Calendar')), React.createElement(Section, {
      label: 'Agent'
    }, React.createElement(Item, {
      icon: ic.Activity({
        size: 16
      })
    }, 'Traces'), React.createElement(Item, {
      icon: ic.Shield({
        size: 16
      }),
      badge: '3'
    }, 'Reviews'), React.createElement(Item, {
      icon: ic.BarChart({
        size: 16
      })
    }, 'Analytics')), React.createElement(Section, {
      label: 'Knowledge'
    }, React.createElement(Item, {
      icon: ic.Database({
        size: 16
      })
    }, 'Sources'), React.createElement(Item, {
      icon: ic.FileText({
        size: 16
      })
    }, 'Documents')));
    return React.createElement('div', {
      'data-theme': theme,
      style: {
        display: 'flex',
        height: '100vh',
        background: 'var(--bg-base)',
        color: 'var(--text-primary)',
        fontFamily: 'var(--font-sans)'
      }
    }, React.createElement(ds.Sidebar, {
      brand: React.createElement(Wordmark),
      footer: React.createElement('div', {
        style: {
          display: 'flex',
          alignItems: 'center',
          gap: 9,
          padding: '4px 8px'
        }
      }, React.createElement('div', {
        style: {
          width: 26,
          height: 26,
          borderRadius: '50%',
          background: 'var(--surface-3)',
          border: '1px solid var(--border-default)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'var(--text-secondary)'
        }
      }, ic.User({
        size: 15
      })), React.createElement('div', {
        style: {
          flex: 1,
          minWidth: 0
        }
      }, React.createElement('div', {
        style: {
          fontSize: 'var(--fs-body-sm)',
          fontWeight: 500,
          whiteSpace: 'nowrap',
          overflow: 'hidden',
          textOverflow: 'ellipsis'
        }
      }, 'Acme Plumbing'), React.createElement('div', {
        style: {
          fontSize: 'var(--fs-micro)',
          color: 'var(--text-tertiary)'
        }
      }, 'Pro · self-hosted')))
    }, nav(ds.SidebarItem, ds.SidebarSection)), React.createElement('div', {
      style: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
        minWidth: 0
      }
    }, React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        height: 'var(--topbar-height)',
        padding: '0 20px',
        borderBottom: '1px solid var(--border-subtle)',
        flexShrink: 0
      }
    }, React.createElement('div', null, React.createElement('div', {
      style: {
        fontSize: 'var(--fs-body)',
        fontWeight: 600,
        letterSpacing: 'var(--ls-normal)'
      }
    }, 'Dashboard')), React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 10
      }
    }, React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 7,
        padding: '5px 10px',
        background: 'var(--status-success-bg)',
        border: '1px solid var(--status-success-border)',
        borderRadius: 'var(--radius-full)'
      }
    }, React.createElement('span', {
      style: {
        width: 6,
        height: 6,
        borderRadius: '50%',
        background: 'var(--status-success-fg)'
      }
    }), React.createElement('span', {
      style: {
        fontSize: 'var(--fs-caption)',
        color: 'var(--status-success-fg)',
        fontWeight: 500
      }
    }, 'Agent online')), React.createElement(ds.IconButton, {
      label: 'Toggle theme',
      icon: ic.Sparkles({
        size: 16
      }),
      onClick: () => setTheme(theme === 'dark' ? 'light' : 'dark')
    }), React.createElement(ds.IconButton, {
      label: 'Notifications',
      icon: ic.Bell({
        size: 16
      })
    }), React.createElement(ds.Button, {
      leadingIcon: ic.Plus({
        size: 15
      })
    }, 'New agent'))), React.createElement('div', {
      style: {
        flex: 1,
        display: 'flex',
        minHeight: 0
      }
    }, React.createElement('div', {
      style: {
        flex: 1,
        overflowY: 'auto',
        padding: 20,
        minWidth: 0
      }
    }, React.createElement('div', {
      style: {
        display: 'flex',
        gap: 14,
        marginBottom: 18
      }
    }, React.createElement(Kpi, {
      label: 'Bookings today',
      value: '38',
      delta: '+12%',
      tone: 'up'
    }), React.createElement(Kpi, {
      label: 'Deflection rate',
      value: '84%',
      delta: '+3pt',
      tone: 'up'
    }), React.createElement(Kpi, {
      label: 'Avg. latency',
      value: '1.3s',
      delta: '−0.2s',
      tone: 'up'
    }), React.createElement(Kpi, {
      label: 'Needs review',
      value: '3',
      delta: '+2',
      tone: 'down'
    })), React.createElement(ds.Card, {
      padding: 'none'
    }, React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '12px 16px 0'
      }
    }, React.createElement(ds.Tabs, {
      label: 'Run views',
      tabs: [{
        value: 'runs',
        label: 'Runs',
        count: runs.length
      }, {
        value: 'review',
        label: 'Needs review',
        count: 3
      }, {
        value: 'all',
        label: 'All channels'
      }],
      value: tab,
      onChange: setTab
    }), React.createElement('div', {
      style: {
        display: 'flex',
        gap: 8,
        paddingBottom: 8
      }
    }, React.createElement(ds.IconButton, {
      label: 'Filter',
      icon: ic.Filter({
        size: 16
      }),
      variant: 'secondary'
    }), React.createElement(ds.IconButton, {
      label: 'Refresh',
      icon: ic.Refresh({
        size: 16
      }),
      variant: 'secondary'
    }))), React.createElement('div', {
      id: `panel-${tab}`,
      role: 'tabpanel',
      'aria-labelledby': `tab-${tab}`,
      style: {
        padding: 16
      }
    }, React.createElement(ds.DataTable, {
      columns: [{
        key: 'id',
        header: 'Run',
        mono: true,
        width: 120
      }, {
        key: 'intent',
        header: 'Intent',
        sortable: true
      }, {
        key: 'channel',
        header: 'Channel'
      }, {
        key: 'conf',
        header: 'Conf.',
        mono: true,
        align: 'right',
        width: 70,
        sortable: true,
        render: v => v.toFixed(2)
      }, {
        key: 'status',
        header: 'Status',
        width: 130,
        render: v => React.createElement(ds.Badge, {
          tone: toneMap[v][0],
          dot: true
        }, toneMap[v][1])
      }, {
        key: 'latency',
        header: 'Latency',
        mono: true,
        align: 'right',
        width: 80
      }, {
        key: 'when',
        header: 'Time',
        mono: true,
        align: 'right',
        width: 64
      }],
      rows: tab === 'review' ? runs.filter(r => r.status === 'review') : runs,
      onRowClick: openTrace
    })))), openRun && React.createElement(TracePanel, {
      run: openRun,
      open: !!openRun,
      onClose: closeTrace
    }))));
  }
  window.KKKit = window.KKKit || {};
  window.KKKit.OperatorDashboard = OperatorDashboard;
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/operator-dashboard/OperatorDashboard.jsx", error: String((e && e.message) || e) }); }

// ui_kits/week-calendar/WeekCalendarScreen.jsx
try { (() => {
/* Kontor week calendar — scheduling screen assembled from the new
   structural + overlay + identity components. Reads DS namespace + icons
   lazily at render (never at module top level). */
(function () {
  const React = window.React;
  const {
    useState
  } = React;
  const DS = () => window.KontorKanonDesignSystem_452420;
  const I = () => window.KKIcons;
  const staff = [{
    id: 's1',
    name: 'Mira Voss'
  }, {
    id: 's2',
    name: 'Theo Lindqvist'
  }, {
    id: 's3',
    name: 'Ada Kessler'
  }];
  const monday = (() => {
    const d = new Date(2026, 6, 20);
    const wd = (d.getDay() + 6) % 7;
    d.setDate(d.getDate() - wd);
    return d;
  })();
  const seedAppointments = [{
    id: 'a1',
    staffId: 's1',
    title: 'Boiler service \u2014 J. Aas',
    start: '09:00',
    end: '10:00',
    status: 'confirmed',
    customer: 'Jonas Aas',
    service: 'Boiler service',
    notes: 'Recurring annual check.'
  }, {
    id: 'a2',
    staffId: 's1',
    title: 'Leak check \u2014 R. Holm',
    start: '10:15',
    end: '11:00',
    status: 'rescheduled',
    customer: 'Ruth Holm',
    service: 'Leak inspection',
    notes: 'Moved from Monday.'
  }, {
    id: 'a3',
    staffId: 's2',
    title: 'Install \u2014 K. Berg',
    start: '09:30',
    end: '11:30',
    status: 'confirmed',
    customer: 'Karin Berg',
    service: 'Water heater install',
    notes: ''
  }, {
    id: 'a4',
    staffId: 's2',
    title: 'Overlap job \u2014 N. Vik',
    start: '10:00',
    end: '10:45',
    status: 'confirmed',
    customer: 'Nils Vik',
    service: 'Drain clearing',
    notes: ''
  }, {
    id: 'a5',
    staffId: 's3',
    title: 'Cancelled \u2014 P. Dahl',
    start: '13:00',
    end: '14:00',
    status: 'cancelled',
    customer: 'Petra Dahl',
    service: 'Pipe replacement',
    notes: 'Customer cancelled.'
  }, {
    id: 'a6',
    staffId: 's3',
    title: 'No-show \u2014 E. Lund',
    start: '15:00',
    end: '15:30',
    status: 'no-show',
    customer: 'Erik Lund',
    service: 'Quote visit',
    notes: ''
  }];
  let idSeq = 100;
  function Wordmark() {
    return React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 9
      }
    }, React.createElement('div', {
      style: {
        width: 26,
        height: 26,
        borderRadius: 7,
        background: 'var(--accent)',
        color: '#fff',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontWeight: 600,
        fontSize: 'var(--fs-caption)',
        letterSpacing: 'var(--ls-tighter)'
      }
    }, 'K&K'), React.createElement('span', {
      style: {
        fontWeight: 600,
        fontSize: 'var(--fs-body)',
        letterSpacing: 'var(--ls-tight)',
        color: 'var(--text-primary)'
      }
    }, 'Kontor ', React.createElement('span', {
      style: {
        color: 'var(--text-tertiary)',
        fontWeight: 400
      }
    }, '&'), ' DocMind'));
  }
  function AppointmentDrawer({
    ds,
    appt,
    open,
    onClose,
    onCancelRequest
  }) {
    return React.createElement(ds.Drawer, {
      open,
      onClose,
      title: 'Appointment',
      size: 'sm',
      footer: appt && appt.status !== 'cancelled' ? React.createElement(ds.Button, {
        variant: 'danger',
        onClick: () => onCancelRequest(appt)
      }, 'Cancel booking') : null
    }, appt && React.createElement(ds.KeyValueList, {
      items: [{
        label: 'Customer',
        value: appt.customer
      }, {
        label: 'Service',
        value: appt.service
      }, {
        label: 'Time',
        value: appt.start + '\u2013' + appt.end,
        mono: true
      }, {
        label: 'Status',
        value: React.createElement(ds.Badge, {
          tone: appt.status === 'confirmed' ? 'success' : appt.status === 'rescheduled' ? 'warning' : appt.status === 'no-show' ? 'error' : 'neutral',
          dot: true
        }, appt.status)
      }, {
        label: 'Notes',
        value: appt.notes || '\u2014'
      }]
    }));
  }
  function NewBookingDrawer({
    ds,
    slot,
    open,
    onClose,
    onCreate
  }) {
    const [name, setName] = useState('');
    const [service, setService] = useState('Boiler service');
    if (!slot) return React.createElement(ds.Drawer, {
      open,
      onClose,
      title: 'New booking',
      size: 'sm'
    });
    const staffName = staff.find(s => s.id === slot.staffId)?.name;
    const hh = String(Math.floor(slot.min / 60)).padStart(2, '0'),
      mm = String(slot.min % 60).padStart(2, '0');
    return React.createElement(ds.Drawer, {
      open,
      onClose,
      title: 'New booking',
      size: 'sm',
      footer: React.createElement(ds.Button, {
        onClick: () => {
          onCreate({
            name,
            service,
            hh,
            mm
          });
        },
        disabled: !name.trim()
      }, 'Create booking')
    }, React.createElement('div', {
      style: {
        display: 'flex',
        flexDirection: 'column',
        gap: 14
      }
    }, React.createElement(ds.KeyValueList, {
      items: [{
        label: 'Staff',
        value: staffName
      }, {
        label: 'Start',
        value: hh + ':' + mm,
        mono: true
      }]
    }), React.createElement(ds.Input, {
      label: 'Customer name',
      placeholder: 'e.g. Sofie Lang',
      value: name,
      onChange: e => setName(e.target.value)
    }), React.createElement(ds.Select, {
      label: 'Service',
      value: service,
      onChange: e => setService(e.target.value),
      options: ['Boiler service', 'Leak inspection', 'Drain clearing', 'Water heater install', 'Quote visit']
    })));
  }
  function WeekCalendarScreen() {
    const ds = DS();
    const ic = I();
    const [day, setDay] = useState(new Date(2026, 6, 20));
    const [appointments, setAppointments] = useState(seedAppointments);
    const [detailAppt, setDetailAppt] = useState(null);
    const [confirmCancel, setConfirmCancel] = useState(null);
    const [newSlot, setNewSlot] = useState(null);
    const [toasts, setToasts] = useState([]);
    const pushToast = t => setToasts(ts => [...ts, {
      id: 't' + idSeq++,
      ...t
    }]);
    const dismissToast = id => setToasts(ts => ts.filter(t => t.id !== id));
    const openAppt = a => setDetailAppt(a);
    const requestCancel = a => {
      setDetailAppt(null);
      setConfirmCancel(a);
    };
    const confirmCancelNow = () => {
      setAppointments(as => as.map(a => a.id === confirmCancel.id ? {
        ...a,
        status: 'cancelled'
      } : a));
      setConfirmCancel(null);
      pushToast({
        tone: 'success',
        title: 'Booking cancelled',
        description: confirmCancel.customer + ' has been notified.'
      });
    };
    const nav = (Item, Section) => React.createElement(React.Fragment, null, React.createElement(Section, {
      label: 'Overview'
    }, React.createElement(Item, {
      icon: ic.Home({
        size: 16
      })
    }, 'Dashboard'), React.createElement(Item, {
      icon: ic.Inbox({
        size: 16
      }),
      badge: '12'
    }, 'Inbox'), React.createElement(Item, {
      icon: ic.Calendar({
        size: 16
      }),
      active: true
    }, 'Calendar')), React.createElement(Section, {
      label: 'Agent'
    }, React.createElement(Item, {
      icon: ic.Activity({
        size: 16
      })
    }, 'Traces'), React.createElement(Item, {
      icon: ic.Shield({
        size: 16
      }),
      badge: '3'
    }, 'Reviews'), React.createElement(Item, {
      icon: ic.BarChart({
        size: 16
      })
    }, 'Analytics')));
    return React.createElement('div', {
      'data-theme': 'dark',
      style: {
        display: 'flex',
        height: '100vh',
        background: 'var(--bg-base)',
        color: 'var(--text-primary)',
        fontFamily: 'var(--font-sans)'
      }
    }, React.createElement(ds.Sidebar, {
      brand: React.createElement(Wordmark),
      footer: React.createElement('div', {
        style: {
          display: 'flex',
          alignItems: 'center',
          gap: 9,
          padding: '4px 8px'
        }
      }, React.createElement(ds.Avatar, {
        name: 'Acme Plumbing',
        size: 'sm'
      }), React.createElement('div', {
        style: {
          flex: 1,
          minWidth: 0
        }
      }, React.createElement('div', {
        style: {
          fontSize: 'var(--fs-body-sm)',
          fontWeight: 500,
          whiteSpace: 'nowrap',
          overflow: 'hidden',
          textOverflow: 'ellipsis'
        }
      }, 'Acme Plumbing'), React.createElement('div', {
        style: {
          fontSize: 'var(--fs-micro)',
          color: 'var(--text-tertiary)'
        }
      }, 'Pro \u00b7 self-hosted')))
    }, nav(ds.SidebarItem, ds.SidebarSection)), React.createElement('div', {
      style: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
        minWidth: 0
      }
    }, React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        height: 'var(--topbar-height)',
        padding: '0 20px',
        borderBottom: '1px solid var(--border-subtle)',
        flexShrink: 0
      }
    }, React.createElement('div', {
      style: {
        fontSize: 'var(--fs-body)',
        fontWeight: 600,
        letterSpacing: 'var(--ls-normal)'
      }
    }, 'Calendar'), React.createElement('div', {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 8
      }
    }, React.createElement(ds.Tooltip, {
      label: 'Staff on shift today'
    }, React.createElement(ds.AvatarGroup, {
      items: staff,
      size: 'sm'
    })), React.createElement(ds.Button, {
      leadingIcon: ic.Plus({
        size: 15
      })
    }, 'New booking'))), React.createElement('div', {
      style: {
        flex: 1,
        overflow: 'auto',
        padding: 20,
        display: 'flex',
        gap: 20,
        alignItems: 'flex-start'
      }
    }, React.createElement('div', {
      style: {
        flex: 1,
        minWidth: 0
      }
    }, React.createElement(ds.WeekCalendar, {
      weekStart: monday,
      selectedDay: day,
      onSelectDay: setDay,
      now: new Date(2026, 6, 20, 11, 20),
      staff,
      workingHours: {
        start: '08:00',
        end: '18:00'
      },
      appointments,
      onAppointmentClick: openAppt,
      onSlotClick: (staffId, min) => setNewSlot({
        staffId,
        min
      })
    })), React.createElement(ds.MonthPicker, {
      month: new Date(2026, 6, 1),
      selected: day,
      today: new Date(2026, 6, 20),
      onSelect: setDay
    }))), React.createElement(AppointmentDrawer, {
      ds,
      appt: detailAppt,
      open: !!detailAppt,
      onClose: () => setDetailAppt(null),
      onCancelRequest: requestCancel
    }), React.createElement(ds.Modal, {
      open: !!confirmCancel,
      onClose: () => setConfirmCancel(null),
      tone: 'destructive',
      title: 'Cancel this booking?',
      description: confirmCancel ? confirmCancel.customer + ' will be notified automatically.' : '',
      primaryLabel: 'Cancel booking',
      onPrimary: confirmCancelNow,
      onSecondary: () => setConfirmCancel(null)
    }), React.createElement(NewBookingDrawer, {
      ds,
      slot: newSlot,
      open: !!newSlot,
      onClose: () => setNewSlot(null),
      onCreate: ({
        name,
        service,
        hh,
        mm
      }) => {
        const startMin = newSlot.min,
          endMin = startMin + 60;
        const fmt = m => String(Math.floor(m / 60)).padStart(2, '0') + ':' + String(m % 60).padStart(2, '0');
        setAppointments(as => [...as, {
          id: 'a' + idSeq++,
          staffId: newSlot.staffId,
          title: service + ' \u2014 ' + name,
          start: fmt(startMin),
          end: fmt(endMin),
          status: 'confirmed',
          customer: name,
          service,
          notes: ''
        }]);
        setNewSlot(null);
        pushToast({
          tone: 'success',
          title: 'Booking created',
          description: name + ' \u00b7 ' + hh + ':' + mm
        });
      }
    }), React.createElement(ds.ToastStack, {
      toasts,
      onDismiss: dismissToast
    }));
  }
  window.KKKit = window.KKKit || {};
  window.KKKit.WeekCalendarScreen = WeekCalendarScreen;
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/week-calendar/WeekCalendarScreen.jsx", error: String((e && e.message) || e) }); }

__ds_ns.Sparkline = __ds_scope.Sparkline;

__ds_ns.BarChart = __ds_scope.BarChart;

__ds_ns.DonutChart = __ds_scope.DonutChart;

__ds_ns.Chart = __ds_scope.Chart;

__ds_ns.Badge = __ds_scope.Badge;

__ds_ns.Button = __ds_scope.Button;

__ds_ns.Card = __ds_scope.Card;

__ds_ns.CardHeader = __ds_scope.CardHeader;

__ds_ns.IconButton = __ds_scope.IconButton;

__ds_ns.CodeBlock = __ds_scope.CodeBlock;

__ds_ns.DataTable = __ds_scope.DataTable;

__ds_ns.KeyValue = __ds_scope.KeyValue;

__ds_ns.KeyValueList = __ds_scope.KeyValueList;

__ds_ns.EmptyState = __ds_scope.EmptyState;

__ds_ns.ErrorState = __ds_scope.ErrorState;

__ds_ns.Skeleton = __ds_scope.Skeleton;

__ds_ns.SkeletonText = __ds_scope.SkeletonText;

__ds_ns.SkeletonRow = __ds_scope.SkeletonRow;

__ds_ns.Checkbox = __ds_scope.Checkbox;

__ds_ns.Input = __ds_scope.Input;

__ds_ns.Select = __ds_scope.Select;

__ds_ns.Switch = __ds_scope.Switch;

__ds_ns.Avatar = __ds_scope.Avatar;

__ds_ns.AvatarGroup = __ds_scope.AvatarGroup;

__ds_ns.Sidebar = __ds_scope.Sidebar;

__ds_ns.SidebarSection = __ds_scope.SidebarSection;

__ds_ns.SidebarItem = __ds_scope.SidebarItem;

__ds_ns.Tabs = __ds_scope.Tabs;

__ds_ns.Drawer = __ds_scope.Drawer;

__ds_ns.Modal = __ds_scope.Modal;

__ds_ns.Toast = __ds_scope.Toast;

__ds_ns.ToastStack = __ds_scope.ToastStack;

__ds_ns.Tooltip = __ds_scope.Tooltip;

__ds_ns.WeekCalendar = __ds_scope.WeekCalendar;

__ds_ns.DayCalendar = __ds_scope.DayCalendar;

__ds_ns.MonthPicker = __ds_scope.MonthPicker;

__ds_ns.Calendar = __ds_scope.Calendar;

__ds_ns.Timeline = __ds_scope.Timeline;

__ds_ns.TimelineSkeleton = __ds_scope.TimelineSkeleton;

})();
