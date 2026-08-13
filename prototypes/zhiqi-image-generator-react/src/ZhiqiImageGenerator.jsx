import { useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeft,
  CaretDown,
  Check,
  CheckCircle,
  CornersOut,
  Cube,
  DeviceMobile,
  ImageSquare,
  MagicWand,
  Plus,
  Question,
  Rectangle,
  Square,
  Sparkle,
  SpinnerGap,
  Stack,
  Trash,
  WarningCircle,
} from "@phosphor-icons/react";

const ASPECT_OPTIONS = [
  { id: "auto", label: "auto", icon: CornersOut },
  { id: "1:1", label: "1:1", icon: Square },
  { id: "16:9", label: "16:9", icon: Rectangle },
  { id: "9:16", label: "9:16", icon: DeviceMobile },
  { id: "4:3", label: "4:3", icon: Rectangle },
];

const MODEL_OPTIONS = ["GPT Image 2", "GPT Image 1.5", "Seedream 4.0"];
const COUNT_OPTIONS = [1, 2, 4];

const cx = (...classes) => classes.filter(Boolean).join(" ");

function OptionButton({ active, children, className = "", ...props }) {
  return (
    <button
      type="button"
      aria-pressed={active}
      className={cx(
        "relative min-h-11 rounded-xl border px-3 text-sm font-semibold transition",
        "focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-zhiqi-purple/25",
        active
          ? "border-zhiqi-purple bg-zhiqi-purple-soft text-zhiqi-purple shadow-sm"
          : "border-zhiqi-line bg-white text-zhiqi-ink hover:border-zhiqi-purple/55 hover:bg-zhiqi-purple-soft/50",
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

function SelectField({ icon: Icon, label, value, options, onChange }) {
  return (
    <label className="group flex min-h-[72px] items-center gap-3 rounded-card border border-zhiqi-line bg-white px-4 transition hover:border-zhiqi-purple/45 focus-within:border-zhiqi-purple focus-within:ring-4 focus-within:ring-zhiqi-purple/15">
      <Icon aria-hidden="true" className="shrink-0 text-zhiqi-purple" size={28} weight="duotone" />
      <span className="min-w-0 flex-1">
        <span className="block text-xs font-medium text-zhiqi-muted">{label}</span>
        <span className="mt-1 block truncate font-display text-sm font-bold text-zhiqi-ink">{value}</span>
      </span>
      <CaretDown aria-hidden="true" className="text-zhiqi-muted transition group-focus-within:rotate-180" size={18} />
      <select
        aria-label={label}
        className="absolute h-px w-px opacity-0"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
    </label>
  );
}

export function ZhiqiImageGenerator({
  initialPrompt = "例如：生成一张水果店开业促销海报，橙色系，高级感",
  onBack,
  onGenerate,
}) {
  const fileInputRef = useRef(null);
  const previewUrlsRef = useRef([]);
  const [prompt, setPrompt] = useState(initialPrompt);
  const [aspectRatio, setAspectRatio] = useState("auto");
  const [resolution, setResolution] = useState("1K");
  const [model, setModel] = useState(MODEL_OPTIONS[0]);
  const [count, setCount] = useState(1);
  const [referenceImages, setReferenceImages] = useState([]);
  const [status, setStatus] = useState({ tone: "idle", message: "" });
  const [isGenerating, setIsGenerating] = useState(false);

  useEffect(
    () => () => {
      previewUrlsRef.current.forEach((url) => URL.revokeObjectURL(url));
    },
    [],
  );

  const estimatedPoints = useMemo(() => {
    const resolutionMultiplier = resolution === "2K" ? 1.8 : 1;
    return Math.round(10 * count * resolutionMultiplier);
  }, [count, resolution]);

  const handleImage = (event) => {
    const files = Array.from(event.target.files || []);
    event.target.value = "";
    if (!files.length) return;
    if (files.some((file) => !file.type.startsWith("image/"))) {
      setStatus({ tone: "error", message: "请选择 PNG、JPG 或 WebP 图片。" });
      return;
    }
    if (files.some((file) => file.size > 5 * 1024 * 1024)) {
      setStatus({ tone: "error", message: "每张参考图不能超过 5MB。" });
      return;
    }
    const remaining = Math.max(0, 3 - referenceImages.length);
    const accepted = files.slice(0, remaining).map((file) => ({ file, previewUrl: URL.createObjectURL(file) }));
    previewUrlsRef.current.push(...accepted.map((item) => item.previewUrl));
    setReferenceImages((current) => [...current, ...accepted]);
    setStatus({
      tone: accepted.length ? "success" : "error",
      message: accepted.length ? `已添加 ${accepted.length} 张参考图。` : "最多添加 3 张参考图。",
    });
  };

  const removeImage = (index) => {
    setReferenceImages((current) => {
      const target = current[index];
      if (target) {
        URL.revokeObjectURL(target.previewUrl);
        previewUrlsRef.current = previewUrlsRef.current.filter((url) => url !== target.previewUrl);
      }
      return current.filter((_, itemIndex) => itemIndex !== index);
    });
    setStatus({ tone: "idle", message: "" });
  };

  const optimizePrompt = () => {
    const nextPhrase = "，突出商品主体，商业摄影质感，留出清晰文案空间";
    setPrompt((current) => (current.includes("商业摄影质感") ? current : `${current.trim()}${nextPhrase}`));
    setStatus({ tone: "success", message: "提示词已优化，你仍可继续编辑。" });
  };

  const generate = async () => {
    if (!prompt.trim()) {
      setStatus({ tone: "error", message: "请先描述想生成的图片。" });
      return;
    }
    setIsGenerating(true);
    setStatus({ tone: "loading", message: "正在创建生成任务…" });
    try {
      await new Promise((resolve) => setTimeout(resolve, 1100));
      await onGenerate?.({
        prompt,
        aspectRatio,
        resolution,
        model,
        count,
        referenceImages: referenceImages.map((item) => item.file),
      });
      setStatus({ tone: "success", message: "任务已提交，生成后可继续编辑和优化。" });
    } catch (error) {
      setStatus({ tone: "error", message: error instanceof Error ? error.message : "提交失败，请稍后重试。" });
    } finally {
      setIsGenerating(false);
    }
  };

  return (
    <section className="min-h-screen bg-zhiqi-canvas font-body text-zhiqi-ink">
      <div className="mx-auto flex min-h-screen w-full max-w-6xl flex-col bg-white shadow-soft lg:my-8 lg:min-h-0 lg:rounded-[28px]">
        <header className="relative flex min-h-[72px] items-center justify-between border-b border-zhiqi-line px-4 sm:px-7">
          <button
            type="button"
            aria-label="返回"
            onClick={onBack}
            className="grid size-11 place-items-center rounded-full text-zhiqi-ink transition hover:bg-zhiqi-purple-soft focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-zhiqi-purple/25"
          >
            <ArrowLeft size={24} weight="bold" />
          </button>
          <div className="absolute left-1/2 -translate-x-1/2 text-center">
            <h1 className="font-display text-xl font-extrabold tracking-tight sm:text-2xl">AI生图</h1>
          </div>
          <button
            type="button"
            aria-label="查看帮助"
            className="grid size-11 place-items-center rounded-full text-zhiqi-ink transition hover:bg-zhiqi-purple-soft focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-zhiqi-purple/25"
          >
            <Question size={24} weight="bold" />
          </button>
        </header>

        <div className="flex items-center justify-center gap-2 px-5 py-4 text-center text-sm text-zhiqi-muted">
          <Sparkle aria-hidden="true" className="text-zhiqi-purple" size={20} weight="duotone" />
          <span>输入想法，AI 将为你生成高质量图片</span>
        </div>

        <main className="flex-1 px-4 pb-32 sm:px-7 lg:px-10 lg:pb-10">
          <div className="mx-auto max-w-5xl">
            <h2 className="mb-4 font-display text-2xl font-extrabold tracking-tight sm:text-3xl">今天想生成什么？</h2>

            <div className="rounded-card border border-zhiqi-purple/40 bg-white shadow-sm focus-within:border-zhiqi-purple focus-within:ring-4 focus-within:ring-zhiqi-purple/15">
              <div className="grid gap-5 p-4 sm:grid-cols-[112px_minmax(0,1fr)] sm:p-5">
                <div>
                  {referenceImages.length ? (
                    <div className="grid grid-cols-2 gap-2" aria-label={`${referenceImages.length} 张参考图`}>
                      {referenceImages.map((image, index) => (
                        <div key={image.previewUrl} className="group relative aspect-square overflow-hidden rounded-xl border border-zhiqi-purple bg-zhiqi-purple-soft">
                          <img className="h-full w-full object-cover" src={image.previewUrl} alt={`参考图 ${index + 1}`} />
                          <button
                            type="button"
                            aria-label={`删除参考图 ${index + 1}`}
                            onClick={() => removeImage(index)}
                            className="absolute right-1 top-1 grid size-8 place-items-center rounded-full bg-white/95 text-red-700 shadow transition hover:bg-red-50 focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-white/80"
                          >
                            <Trash size={16} weight="bold" />
                          </button>
                        </div>
                      ))}
                      {referenceImages.length < 3 ? (
                        <button
                          type="button"
                          aria-label="继续添加参考图"
                          onClick={() => fileInputRef.current?.click()}
                          className="flex aspect-square items-center justify-center rounded-xl border border-dashed border-zhiqi-purple/45 bg-zhiqi-purple-soft/45 text-zhiqi-purple transition hover:border-zhiqi-purple hover:bg-zhiqi-purple-soft focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-zhiqi-purple/25"
                        >
                          <Plus size={24} weight="bold" />
                        </button>
                      ) : null}
                    </div>
                  ) : (
                    <button
                      type="button"
                      onClick={() => fileInputRef.current?.click()}
                      className="flex aspect-square w-full min-w-[96px] flex-col items-center justify-center rounded-card border border-dashed border-zhiqi-purple/45 bg-zhiqi-purple-soft/45 text-zhiqi-purple transition hover:border-zhiqi-purple hover:bg-zhiqi-purple-soft focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-zhiqi-purple/25"
                    >
                      <Plus size={28} weight="bold" />
                      <span className="mt-2 text-sm font-bold">添加参考</span>
                      <span className="mt-1 text-xs text-zhiqi-muted">0/3</span>
                    </button>
                  )}
                  <input ref={fileInputRef} className="sr-only" type="file" accept="image/png,image/jpeg,image/webp" multiple onChange={handleImage} />
                </div>

                <div className="flex min-h-[240px] flex-col sm:min-h-[260px]">
                  <label className="sr-only" htmlFor="zhiqi-prompt">图片描述</label>
                  <textarea
                    id="zhiqi-prompt"
                    value={prompt}
                    maxLength={500}
                    onChange={(event) => {
                      setPrompt(event.target.value);
                      setStatus({ tone: "idle", message: "" });
                    }}
                    className="min-h-[180px] w-full flex-1 resize-none border-0 bg-transparent text-[15px] leading-7 text-zhiqi-ink placeholder:text-zhiqi-muted focus:outline-none sm:text-base"
                    placeholder="例如：生成一张水果店开业促销海报，橙色系，高级感"
                  />
                  <div className="mt-3 flex items-center justify-between gap-3">
                    <button
                      type="button"
                      onClick={optimizePrompt}
                      disabled={!prompt.trim()}
                      className="inline-flex min-h-11 items-center gap-2 rounded-xl border border-zhiqi-purple/35 px-4 text-sm font-bold text-zhiqi-purple transition hover:bg-zhiqi-purple-soft focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-zhiqi-purple/25 disabled:cursor-not-allowed disabled:opacity-45"
                    >
                      <MagicWand size={19} weight="duotone" />
                      帮我优化
                    </button>
                    <span aria-live="polite" className="text-sm text-zhiqi-muted">{prompt.length}/500</span>
                  </div>
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-2 border-t border-zhiqi-line p-3 sm:px-5">
                <OptionButton active={aspectRatio === "auto"} onClick={() => setAspectRatio("auto")} className="inline-flex items-center gap-2">
                  <ImageSquare size={19} weight="duotone" />
                  自动比例
                  <CaretDown size={15} />
                </OptionButton>
                <OptionButton active={resolution === "1K"} onClick={() => setResolution("1K")} className="inline-flex items-center gap-2">
                  1K <CaretDown size={15} />
                </OptionButton>
                <button
                  type="button"
                  onClick={() => setStatus({ tone: "success", message: "当前设置已应用。" })}
                  className="ml-auto min-h-11 rounded-xl bg-zhiqi-purple px-5 text-sm font-bold text-white transition hover:bg-zhiqi-purple-dark focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-zhiqi-purple/30"
                >
                  应用
                </button>
              </div>
            </div>

            <div className="mt-7">
              <h3 className="font-display text-xl font-extrabold">生成设置</h3>
              <fieldset className="mt-5">
                <legend className="mb-3 text-sm font-bold">画幅比例</legend>
                <div className="grid grid-cols-3 gap-2 sm:grid-cols-5">
                  {ASPECT_OPTIONS.map((option) => {
                    const active = aspectRatio === option.id;
                    const AspectIcon = option.icon;
                    return (
                      <OptionButton
                        key={option.id}
                        active={active}
                        onClick={() => setAspectRatio(option.id)}
                        className="flex min-h-[96px] flex-col items-center justify-center gap-2"
                      >
                        {active ? (
                          <span className="absolute right-2 top-2 grid size-5 place-items-center rounded-full bg-zhiqi-purple text-white">
                            <Check size={13} weight="bold" />
                          </span>
                        ) : null}
                        <AspectIcon aria-hidden="true" size={30} weight={active ? "duotone" : "regular"} />
                        <span>{option.label}</span>
                      </OptionButton>
                    );
                  })}
                </div>
              </fieldset>

              <fieldset className="mt-6">
                <legend className="mb-3 text-sm font-bold">图片清晰度</legend>
                <div className="grid grid-cols-2 overflow-hidden rounded-card border border-zhiqi-line bg-white p-1">
                  {["1K", "2K"].map((option) => (
                    <button
                      key={option}
                      type="button"
                      aria-pressed={resolution === option}
                      onClick={() => setResolution(option)}
                      className={cx(
                        "min-h-12 rounded-xl text-sm font-extrabold transition focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-zhiqi-purple/25",
                        resolution === option ? "bg-zhiqi-purple-soft text-zhiqi-purple" : "text-zhiqi-muted hover:bg-zhiqi-canvas hover:text-zhiqi-ink",
                      )}
                    >
                      {option}
                    </button>
                  ))}
                </div>
              </fieldset>

              <div className="mt-6 grid gap-3 sm:grid-cols-2">
                <SelectField icon={Cube} label="模型" value={model} options={MODEL_OPTIONS} onChange={setModel} />
                <SelectField icon={Stack} label="张数" value={count} options={COUNT_OPTIONS} onChange={(value) => setCount(Number(value))} />
              </div>
            </div>

            <div aria-live="polite" className="mt-5 min-h-7">
              {status.message ? (
                <p className={cx("flex items-center gap-2 text-sm font-semibold", status.tone === "error" ? "text-red-700" : "text-zhiqi-purple-dark")}>
                  {status.tone === "loading" ? (
                    <SpinnerGap className="animate-spin" size={18} />
                  ) : status.tone === "error" ? (
                    <WarningCircle size={18} weight="fill" />
                  ) : (
                    <CheckCircle size={18} weight="fill" />
                  )}
                  {status.message}
                </p>
              ) : null}
            </div>
          </div>
        </main>

        <footer className="fixed inset-x-0 bottom-0 z-20 border-t border-zhiqi-line bg-white/95 px-4 py-3 shadow-[0_-8px_28px_rgba(27,24,56,0.08)] backdrop-blur sm:px-7 lg:sticky lg:rounded-b-[28px]">
          <div className="mx-auto flex max-w-5xl items-center justify-between gap-4">
            <div className="min-w-0">
              <p className="text-sm font-semibold text-zhiqi-ink sm:text-base">
                预计 <span className="font-display text-2xl font-extrabold text-[#A54200]">{estimatedPoints}</span> 积分
              </p>
              <p className="hidden text-xs text-zhiqi-muted sm:block">生成后可继续编辑和优化</p>
            </div>
            <button
              type="button"
              disabled={isGenerating || !prompt.trim()}
              onClick={generate}
              className="inline-flex min-h-[52px] min-w-[168px] items-center justify-center gap-2 rounded-card bg-zhiqi-orange px-6 py-3 font-display text-base font-extrabold text-[#231000] shadow-[0_10px_24px_rgba(255,119,27,0.24)] transition hover:bg-[#ED650A] focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-zhiqi-purple/30 active:translate-y-px disabled:cursor-not-allowed disabled:opacity-50 sm:min-w-[230px] sm:text-lg"
            >
              {isGenerating ? <SpinnerGap className="animate-spin" size={23} weight="bold" /> : <Sparkle size={23} weight="fill" />}
              {isGenerating ? "生成中…" : "生成图片"}
            </button>
          </div>
        </footer>
      </div>
    </section>
  );
}
