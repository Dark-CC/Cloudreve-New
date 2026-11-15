import {
  Badge,
  Box,
  Button,
  Card,
  CardActions,
  CardContent,
  Container,
  DialogContent,
  Divider,
  Grid2,
  styled,
  TextField,
  ToggleButton,
  Typography,
} from "@mui/material";
import { useQueryState } from "nuqs";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSnackbar } from "notistack";
import { getCredit, getGiftCode, redeemGiftCode } from "../../../api/api.ts";
import { ProductType } from "../../../api/dashboard.ts";
import { GroupProduct, StorageProduct } from "../../../api/site.ts";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import { loadSiteConfig } from "../../../redux/thunks/site.ts";
import { formatPrice, sizeToString } from "../../../util/index.ts";
import ResponsiveTabs, { Tab } from "../../Common/ResponsiveTabs.tsx";
import { DefaultCloseAction } from "../../Common/Snackbar/snackbar.tsx";
import DraggableDialog from "../../Dialogs/DraggableDialog.tsx";
import PersonStar from "../../Icons/PersonStar.tsx";
import Storage from "../../Icons/Storage.tsx";
import TicketDiagonal from "../../Icons/TicketDiagonal.tsx";
import PageContainer from "../PageContainer.tsx";
import PageHeader, { PageTabQuery } from "../PageHeader.tsx";
import PurchaseDialog from "./PurchaseDialog.tsx";

enum ShopTab {
  Group = "group",
  Storage = "storage",
  Redeem = "redeem",
}

interface PurchaseTarget {
  type: ProductType;
  name: string;
  productID?: string;
  priceCashUnit: number;
  pricePoints: number;
  duration: number;
}

const StorageToggle = styled(ToggleButton)(({ theme }) => ({
  boxShadow: `0 0 0 1px ${theme.palette.divider}`,
  border: "none",
  flexDirection: "column",
  borderRadius: `${theme.shape.borderRadius}px`,
  padding: theme.spacing(0.5, 1),
  width: "100%",
  height: "100%",
  "&.Mui-selected": {
    color: theme.palette.text.secondary,
    boxShadow: `0 0 0 2px ${theme.palette.primary.main}`,
    backgroundColor: `${theme.palette.primary.main}1a`,
    "&:hover": {
      backgroundColor: `${theme.palette.primary.main}33`,
    },
  },
  transition: theme.transitions.create(["background-color", "box-shadow"]),
}));

const StorageProductCard = ({
  product,
  selected,
  onSelect,
}: {
  product: StorageProduct;
  selected: boolean;
  onSelect: () => void;
}) => {
  const { t } = useTranslation();
  const payment = useAppSelector((state) => state.siteConfig.vas?.config?.payment);
  return (
    <Badge
      badgeContent={product.chip}
      color="primary"
      sx={{ width: "100%", "& .MuiBadge-badge": { right: 0, transform: "scale(1) translate(2px, -50%)" } }}
    >
      <StorageToggle value="check" selected={selected} onClick={onSelect}>
        <Typography variant="subtitle2" color="text.primary">
          {product.name}
        </Typography>
        <Typography color="primary" sx={{ py: 1, fontWeight: 600 }} variant="h5">
          {payment ? formatPrice(payment.currency_mark, product.price, payment.currency_unit) : "-"}
        </Typography>
        <Typography variant="subtitle2">
          {sizeToString(product.size)} -{" "}
          {t("application:vas.validDurationDays", { num: Math.ceil((product.time ?? 0) / 86400) })}
        </Typography>
      </StorageToggle>
    </Badge>
  );
};

const GroupProductCard = ({ product, onSelect }: { product: GroupProduct; onSelect: () => void }) => {
  const { t } = useTranslation();
  const payment = useAppSelector((state) => state.siteConfig.vas?.config?.payment);
  return (
    <Badge
      badgeContent={product.chip}
      color="primary"
      sx={{ width: "100%", "& .MuiBadge-badge": { right: 0, transform: "scale(1) translate(2px, -50%)" } }}
    >
      <Card variant="outlined" sx={{ height: "100%", width: "100%" }}>
        <CardContent>
          <Typography variant="h6" sx={{ mb: 1 }} color="textPrimary">
            {product.name}
          </Typography>
          <Box>
            <Typography component="span" variant="h4" color="primary" fontWeight={600}>
              {payment ? formatPrice(payment.currency_mark, product.price, payment.currency_unit) : "-"}
            </Typography>
            <Typography component="span" variant="subtitle1" color="textSecondary" sx={{ ml: 1 }}>
              {t("application:vas.validDurationDays", {
                num: Math.ceil((product.time ?? 0) / 86400),
              })}
            </Typography>
          </Box>
          <Divider sx={{ my: 2 }} />
          <Box>
            {(product.des ?? []).map((line) => (
              <Typography key={line} variant="subtitle1">
                {line}
              </Typography>
            ))}
          </Box>
        </CardContent>
        <CardActions>
          <Button variant="contained" fullWidth onClick={onSelect}>
            {t("application:vas.purchaseNow")}
          </Button>
        </CardActions>
      </Card>
    </Badge>
  );
};

const Redeem = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const { enqueueSnackbar } = useSnackbar();
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [preview, setPreview] = useState<{ name?: string; duration?: number; qyt?: number }>();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const previewGift = () => {
    setLoading(true);
    dispatch(getGiftCode(code))
      .then((res) => {
        setPreview(res);
        setConfirmOpen(true);
      })
      .finally(() => setLoading(false));
  };

  const redeem = () => {
    setLoading(true);
    dispatch(redeemGiftCode(code))
      .then(() => {
        setConfirmOpen(false);
        setCode("");
        enqueueSnackbar({
          variant: "success",
          message: t("application:vas.productDelivered"),
          action: DefaultCloseAction,
        });
        dispatch(getCredit()).catch(() => undefined);
      })
      .finally(() => setLoading(false));
  };

  return (
    <Box sx={{ mt: 2 }}>
      <DraggableDialog
        title={t("application:vas.confirmRedeem")}
        showActions
        loading={loading}
        onAccept={redeem}
        showCancel
        dialogProps={{ open: confirmOpen, onClose: () => setConfirmOpen(false), fullWidth: true, maxWidth: "sm" }}
      >
        <DialogContent sx={{ pb: 0 }}>
          {preview && (
            <Box sx={{ display: "flex" }}>
              <Typography fontWeight={500}>{preview.name}</Typography>
              <Typography sx={{ ml: 2 }} color="text.secondary">
                {preview.duration
                  ? t("application:vas.validDurationDays", {
                      num: Math.ceil((preview.duration * (preview.qyt ?? 1)) / 86400),
                    })
                  : `x ${preview.qyt}`}
              </Typography>
            </Box>
          )}
        </DialogContent>
      </DraggableDialog>
      <Container maxWidth="lg">
        <TextField
          fullWidth
          sx={{ mt: 2 }}
          variant="filled"
          label={t("application:vas.enterGiftCode")}
          autoFocus
          value={code}
          onChange={(event) => setCode(event.target.value)}
          slotProps={{ htmlInput: { style: { textTransform: "uppercase" } } }}
        />
        <Button onClick={previewGift} variant="contained" sx={{ mt: 2 }} disabled={!code || loading}>
          {t("login.continue")}
        </Button>
      </Container>
    </Box>
  );
};

const Shop = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const [tab, setTab] = useQueryState(PageTabQuery);
  const [target, setTarget] = useState<PurchaseTarget>();
  const [purchaseOpen, setPurchaseOpen] = useState(false);
  const storageProducts = useAppSelector((state) => state.siteConfig.vas?.config?.storage_products);
  const groupProducts = useAppSelector((state) => state.siteConfig.vas?.config?.group_skus);

  const tabs = useMemo<Tab<ShopTab>[]>(() => {
    const result: Tab<ShopTab>[] = [];
    if (groupProducts && groupProducts.length > 0) {
      result.push({
        label: t("application:vas.membership"),
        value: ShopTab.Group,
        icon: <PersonStar />,
      });
    }
    if (storageProducts && storageProducts.length > 0) {
      result.push({
        label: t("application:vas.storageExpansion"),
        value: ShopTab.Storage,
        icon: <Storage />,
      });
    }
    result.push({ label: t("application:vas.redeem"), value: ShopTab.Redeem, icon: <TicketDiagonal /> });
    return result;
  }, [groupProducts, storageProducts, t]);

  const effectiveTab = tabs.some((item) => item.value === tab) ? (tab as ShopTab) : tabs[0]?.value;

  useEffect(() => {
    dispatch(loadSiteConfig("vas"));
  }, [dispatch]);

  useEffect(() => {
    if (!tabs.some((item) => item.value === tab) && tabs[0]) {
      setTab(tabs[0].value);
    }
  }, [setTab, tab, tabs]);

  const openPurchase = (next: PurchaseTarget) => {
    setTarget(next);
    setPurchaseOpen(true);
  };

  return (
    <PageContainer>
      <Container maxWidth="lg">
        <PageHeader title={t("application:vas.shop")} />
        <ResponsiveTabs
          value={effectiveTab as ShopTab}
          onChange={(_event, value) => setTab(value)}
          tabs={tabs}
        />
        {effectiveTab === ShopTab.Storage && storageProducts && (
          <Box sx={{ mt: 2 }}>
            <Grid2 container spacing={2}>
              {storageProducts.map((product) => (
                <Grid2 key={product.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
                  <StorageProductCard
                    product={product}
                    selected={target?.productID === product.id && purchaseOpen}
                    onSelect={() =>
                      openPurchase({
                        type: ProductType.storage,
                        name: product.name,
                        productID: product.id,
                        priceCashUnit: product.price,
                        pricePoints: product.points ?? 0,
                        duration: product.time ?? 0,
                      })
                    }
                  />
                </Grid2>
              ))}
            </Grid2>
          </Box>
        )}
        {effectiveTab === ShopTab.Group && groupProducts && (
          <Box sx={{ mt: 2 }}>
            <Grid2 container spacing={2}>
              {groupProducts.map((product) => (
                <Grid2 key={product.id} size={{ xs: 12, sm: 6, md: 4 }}>
                  <GroupProductCard
                    product={product}
                    onSelect={() =>
                      openPurchase({
                        type: ProductType.group,
                        name: product.name,
                        productID: product.id,
                        priceCashUnit: product.price,
                        pricePoints: product.points ?? 0,
                        duration: product.time ?? 0,
                      })
                    }
                  />
                </Grid2>
              ))}
            </Grid2>
          </Box>
        )}
        {effectiveTab === ShopTab.Redeem && <Redeem />}
      </Container>
      <PurchaseDialog
        open={purchaseOpen}
        type={target?.type ?? ProductType.storage}
        name={target?.name ?? ""}
        productId={target?.productID}
        priceCashUnit={target?.priceCashUnit ?? 0}
        pricePoints={target?.pricePoints ?? 0}
        duration={target?.duration ?? 0}
        onClose={() => setPurchaseOpen(false)}
        onPaymentCompleted={() => {
          dispatch(getCredit()).catch(() => undefined);
        }}
      />
    </PageContainer>
  );
};

export default Shop;
